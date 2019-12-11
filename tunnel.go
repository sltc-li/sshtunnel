package sshtunnel

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"golang.org/x/crypto/ssh"
)

type logger interface {
	Printf(string, ...interface{})
}

type tunnel struct {
	auth []ssh.AuthMethod

	gatewayUser string
	gatewayHost string

	dialAddr string
	bindAddr string

	log logger
}

func NewTunnel(
	keyFiles []string,
	gatewayStr string, // user@addr:port
	tunnelStr string,  // remoteAddr:port -> 127.0.0.1:port
	log logger,
) (*tunnel, error) {
	auth, err := parseKeyFiles(keyFiles)
	if err != nil {
		return nil, fmt.Errorf("parse key files: %w", err)
	}
	gatewayInfo := strings.Split(gatewayStr, "@")
	if len(gatewayInfo) != 2 {
		return nil, errors.New("invalid gateway format (e.g. user@addr:port)")
	}
	gatewayUser, gatewayHost := gatewayInfo[0], gatewayInfo[1]
	if _, _, err := net.SplitHostPort(gatewayHost); err != nil {
		gatewayHost += ":22"
	}
	tunnelInfo := strings.Split(tunnelStr, " -> ")
	if len(tunnelInfo) != 2 {
		return nil, errors.New("invalid tunnel format (e.g. remoteAddr:port -> 127.0.0.1:port)")
	}
	return &tunnel{
		auth:        auth,
		gatewayUser: gatewayUser,
		gatewayHost: gatewayHost,
		dialAddr:    tunnelInfo[0],
		bindAddr:    tunnelInfo[1],
		log:         log,
	}, nil
}

func (t *tunnel) Forward(ctx context.Context) {
	ctx, cancel := context.WithCancel(ctx)
	defer cancel()

	sshClient, err := t.dialGateway(ctx)
	if err != nil {
		t.log.Printf("failed to dial gateway %s: %v", t.gatewayHost, err)
		return
	}
	defer sshClient.Close()

	bindListener, err := closableListen("tcp", t.bindAddr)
	if err != nil {
		t.log.Printf("failed to listen %s: %v", t.bindAddr, err)
		return
	}
	defer bindListener.Close()

	t.log.Printf("start forwarding: %s -> %s", t.dialAddr, t.bindAddr)
	defer t.log.Printf("stop forwarding: %s -> %s", t.dialAddr, t.bindAddr)

	t.startAccept(ctx, sshClient, bindListener)
}

func (t *tunnel) startAccept(ctx context.Context, sshClient *ssh.Client, bindListener *closableListener) {
	// close bind listener to stop accepting if ctx is canceled.
	go func() {
		<-ctx.Done()
		bindListener.Close()
	}()

	for {
		bindConn, err := bindListener.Accept()
		if bindListener.IsClosed() {
			break
		}
		if err != nil {
			t.log.Printf("failed to accept %s: %v", t.bindAddr, err)
			break
		}

		t.log.Printf("accepted %s -> %s", t.bindAddr, bindConn.RemoteAddr())
		go func(bindConn net.Conn) {
			defer t.log.Printf("disconnected %s -> %s", t.bindAddr, bindConn.RemoteAddr())

			ctx, cancel := context.WithCancel(ctx)
			defer cancel()
			// ensure to close bind connection when copying finished.
			go func() {
				<-ctx.Done()
				bindConn.Close()
			}()

			dialConn, err := sshClient.Dial("tcp", t.dialAddr)
			if err != nil {
				t.log.Printf("failed to dial %s: %v", t.dialAddr, err)
				return
			}
			// ensure to close dial connection when copying finished.
			go func() {
				<-ctx.Done()
				dialConn.Close()
			}()

			t.biCopy(dialConn, bindConn, cancel)
		}(bindConn)
	}
}

func (t *tunnel) biCopy(dialConn, bindConn net.Conn, shutdown func()) {
	var wg sync.WaitGroup
	wg.Add(2)

	go func() {
		defer wg.Done()
		defer shutdown()
		if _, err := io.Copy(dialConn, bindConn); err != nil {
			t.log.Printf("failed to copy %s -> %s: %v", t.dialAddr, t.bindAddr, err)
		}
	}()

	go func() {
		defer wg.Done()
		defer shutdown()
		if _, err := io.Copy(bindConn, dialConn); err != nil {
			t.log.Printf("failed to copy %s -> %s: %v", t.bindAddr, t.dialAddr, err)
		}
	}()

	wg.Wait()
}

func (t *tunnel) dialGateway(ctx context.Context) (*ssh.Client, error) {
	client, err := ssh.Dial("tcp", t.gatewayHost, &ssh.ClientConfig{
		User:            t.gatewayUser,
		Auth:            t.auth,
		HostKeyCallback: ssh.InsecureIgnoreHostKey(),
		Timeout:         2 * time.Second,
	})
	if err != nil {
		return nil, err
	}
	go t.keepAlive(ctx, client)
	return client, nil
}

func (t *tunnel) keepAlive(ctx context.Context, client *ssh.Client) {
	wait := make(chan error, 1)
	go func() {
		wait <- client.Wait()
	}()

	var aliveErrCount uint32
	ticker := time.NewTicker(30 * time.Second)
	defer ticker.Stop()

	for {
		select {
		case <-wait:
			return
		case <-ticker.C:
			if atomic.LoadUint32(&aliveErrCount) > 1 {
				t.log.Printf("failed to keep alive of %v", client.RemoteAddr())
				client.Close()
				return
			}
		case <-ctx.Done():
			return
		}

		go func() {
			_, _, err := client.SendRequest("keepalive@openssh.com", true, nil)
			if err != nil {
				atomic.AddUint32(&aliveErrCount, 1)
			}
		}()
	}
}
