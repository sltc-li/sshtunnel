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
)

func NewTunnel(gateway *Gateway, logger *log.Logger) *Tunnel {
	return &Tunnel{gateway: gateway, logger: logger, quit: make(chan bool)}
}

type Tunnel struct {
	gateway    *Gateway
	logger     *log.Logger
	mutex      sync.Mutex
	forwarding bool
	quit       chan bool

	url      string
	localURL string
}

func (t *Tunnel) ForwardLocal(url string) error {
	if !strings.Contains(url, ":") {
		return errors.New("no port found in url: " + url)
	}
	port := strings.Split(url, ":")[0]
	return t.Forward(url, "localhost:"+port)
}

func (t *Tunnel) Forward(url string, localURL string) error {
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

	errCh := make(chan error)

	go func() {
		errCh <- t.forward()
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

func (t *Tunnel) Stop() error {
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

func (t *Tunnel) forward() error {
	localListener, err := net.Listen("tcp", t.localURL)
	if err != nil {
		return errors.Wrap(err, "failed to listen "+t.localURL)
	}
	defer localListener.Close()

	t.logger.Printf("start forwarding from %s to %s ...\n", t.url, t.localURL)

	errCh := make(chan error)

	go func() {
		err := t.startAccept(localListener)
		if err != nil {
			errCh <- errors.Wrap(err, "failed to start to accept")
		}
	}()

	return <-errCh
}

func (t *Tunnel) startAccept(localListener net.Listener) error {
	errCh := make(chan error)

	go func() {
		for {
			localConn, err := localListener.Accept()
			if err != nil {
				errCh <- errors.Wrap(err, "failed to accept "+t.localURL)
				break
			}

			t.logger.Printf("accepted %s -> %s\n", t.localURL, localConn.RemoteAddr())
			go func(localConn net.Conn) {
				defer localConn.Close()

				sshClient, err := t.gateway.Dial()
				if err != nil {
					errCh <- errors.Wrapf(err, "failed to dial gateway: %s", t.gateway.url)
					return
				}
				defer sshClient.Close()

				conn, err := sshClient.Dial("tcp", t.url)
				if err != nil {
					errCh <- errors.Wrapf(err, "failed to dial: %s", t.url)
					return
				}
				defer conn.Close()

				errCh <- t.biCopy(conn, localConn)
				t.logger.Printf("disconnected %s -> %s\n", t.localURL, localConn.RemoteAddr())
			}(localConn)
		}
	}()

	return <-errCh
}

func (t *Tunnel) biCopy(conn, localConn net.Conn) error {
	errCh := make(chan error)

	go func() {
		errCh <- errors.Wrap(t.copy(conn, localConn), "failed to copy from remote to local")
	}()

	go func() {
		errCh <- errors.Wrap(t.copy(localConn, conn), "failed to copy from local to remote")
	}()

	return <-errCh
}

func (*Tunnel) copy(dst io.Writer, src io.Reader) error {
	_, err := io.Copy(dst, src)
	if err == io.EOF {
		return nil
	}
	if opErr, ok := err.(*net.OpError); ok {
		if sysErr, ok := opErr.Err.(*os.SyscallError); ok {
			if sysErr.Err == syscall.ECONNRESET {
				return nil
			}
		}
	}
	return err
}
