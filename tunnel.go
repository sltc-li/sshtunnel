package sshtunnel

import (
	"io"
	"log"
	"net"
	"os"
	"os/signal"
	"strings"
	"sync"
	"syscall"
	"time"

	"github.com/pkg/errors"
	"golang.org/x/crypto/ssh"
)

type Tunnel interface {
	ForwardLocal(url string) error
	Forward(url, localURL string) error
	Stop() error
}

func NewTunnel(gateway Gateway, logger *log.Logger) Tunnel {
	return &tunnel{gateway: gateway, logger: logger, quit: make(chan bool)}
}

type tunnel struct {
	gateway    Gateway
	logger     *log.Logger
	mutex      sync.Mutex
	forwarding bool
	quit       chan bool

	url      string
	localURL string
}

func (t *tunnel) ForwardLocal(url string) error {
	if !strings.Contains(url, ":") {
		return errors.New("no port found in url: " + url)
	}
	port := strings.Split(url, ":")[0]
	return t.Forward(url, "localhost:"+port)
}

func (t *tunnel) Forward(url string, localURL string) error {
	if err := func() error {
		t.mutex.Lock()
		defer t.mutex.Unlock()
		if t.forwarding {
			return errors.Errorf("already forwarding at: %s:%s", t.localURL, t.url)
		}
		t.forwarding = true
		t.url, t.localURL = url, localURL
		return nil
	}(); err != nil {
		return err
	}

	sshClient, err := t.gateway.Dial()
	if err != nil {
		return errors.Wrap(err, "failed to dial gateway")
	}
	defer sshClient.Close()

	errCh := make(chan error)

	go func() {
		errCh <- t.forward(sshClient)
	}()

	sign := make(chan os.Signal)
	signal.Notify(sign, syscall.SIGINT, syscall.SIGTERM, syscall.SIGKILL)

	select {
	case err := <-errCh:
		t.forwarding = false
		return err
	case <-t.quit:
		return nil
	case <-sign:
		return nil
	}
}

func (t *tunnel) Stop() error {
	func() {
		t.mutex.Lock()
		defer t.mutex.Unlock()
		t.forwarding = false
		t.logger.Printf("stop forwarding from %s to %s ...\n", t.url, t.localURL)
	}()

	timeoutCh := time.After(time.Second)
	select {
	case <-timeoutCh:
		return errors.New("timeout to stop")
	case t.quit <- true:
		return nil
	}
}

func (t *tunnel) forward(sshClient *ssh.Client) error {
	localListener, err := net.Listen("tcp", t.localURL)
	if err != nil {
		return errors.Wrap(err, "failed to listen "+t.localURL)
	}
	defer localListener.Close()

	t.logger.Printf("start forwarding from %s to %s ...\n", t.url, t.localURL)

	errCh := make(chan error)

	go func() {
		err := t.startAccept(localListener, sshClient)
		if err != nil {
			errCh <- errors.Wrap(err, "failed to start to accept")
		}
	}()

	return <-errCh
}

func (t *tunnel) startAccept(localListener net.Listener, sshClient *ssh.Client) error {
	errCh := make(chan error)

	go func() {
		for {
			localConn, err := localListener.Accept()
			if err != nil {
				errCh <- errors.Wrap(err, "failed to accept "+t.localURL)
				break
			}

			t.logger.Printf("accepted %s\n", localConn.RemoteAddr())
			go func(localConn net.Conn) {
				defer localConn.Close()
				conn, err := sshClient.Dial("tcp", t.url)
				if err != nil {
					errCh <- errors.Wrap(err, "failed to dial "+t.url)
				}
				defer conn.Close()
				errCh <- t.biCopy(conn, localConn)
				t.logger.Printf("disconnected %s\n", localConn.RemoteAddr())
			}(localConn)
		}
	}()

	return <-errCh
}

func (t *tunnel) biCopy(conn, localConn net.Conn) error {
	errCh := make(chan error)

	go func() {
		_, err := io.Copy(conn, localConn)
		if err != nil {
			err = errors.Wrap(err, "failed to copy from remote to local")
		}
		errCh <- err
	}()

	go func() {
		_, err := io.Copy(localConn, conn)
		if err != nil {
			err = errors.Wrap(err, "failed to copy from local to remote")
		}
		errCh <- err
	}()

	return <-errCh
}
