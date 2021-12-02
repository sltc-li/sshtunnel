package sshtunnel

import (
	"context"
	"fmt"
	"log"
	"net"
	"os"
	"os/exec"
	"strings"
	"sync"
	"sync/atomic"
	"syscall"
	"time"

	"golang.org/x/crypto/ssh"
)

type sshClientWrapper interface {
	ssh.Conn
	Dial(n, addr string) (net.Conn, error)
}

type Dialer interface {
	Dial() (sshClientWrapper, error)
}

type tcpDialer struct {
	host   string
	config *ssh.ClientConfig
}

func newTCPDialer(host string, config *ssh.ClientConfig) *tcpDialer {
	return &tcpDialer{
		host:   host,
		config: config,
	}
}

func (d *tcpDialer) Dial() (sshClientWrapper, error) {
	return ssh.Dial("tcp", d.host, d.config)
}

type proxyDialer struct {
	host         string
	config       *ssh.ClientConfig
	proxyCommand string
}

func newProxyDialer(host string, config *ssh.ClientConfig, proxyCommand string) *proxyDialer {
	addr, port, _ := net.SplitHostPort(host)
	proxyCommand = strings.Replace(proxyCommand, "%h", addr, -1)
	proxyCommand = strings.Replace(proxyCommand, "%p", port, -1)
	return &proxyDialer{
		host:         host,
		config:       config,
		proxyCommand: proxyCommand,
	}
}

func (d *proxyDialer) Dial() (sshClientWrapper, error) {
	clientConn, proxyConn := net.Pipe()
	cmd := exec.Command("bash", "-c", d.proxyCommand)
	cmd.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}

	cmd.Stdin = proxyConn
	cmd.Stdout = proxyConn
	cmd.Stderr = os.Stderr

	if err := cmd.Start(); err != nil {
		return nil, fmt.Errorf("start proxy command: %w", err)
	}

	conn, incomingChannels, incomingRequests, err := ssh.NewClientConn(clientConn, d.host, d.config)
	if err != nil {
		return nil, fmt.Errorf("create ssh conn via proxy: %w", err)
	}

	client := ssh.NewClient(conn, incomingChannels, incomingRequests)

	return proxySSHClient{
		cmd:    cmd,
		Client: client,
	}, nil
}

type proxySSHClient struct {
	cmd *exec.Cmd
	*ssh.Client
}

func (c proxySSHClient) Close() error {
	err := c.Client.Close()
	_ = syscall.Kill(-c.cmd.Process.Pid, syscall.SIGKILL)
	return err
}

func newReconnectableSSHClient(dialer Dialer) (*reconnectableSSHClient, error) {
	c, err := dialer.Dial()
	if err != nil {
		return nil, err
	}
	return &reconnectableSSHClient{dialer: dialer, c: c}, nil
}

type reconnectableSSHClient struct {
	dialer Dialer

	mux sync.RWMutex
	c   sshClientWrapper
}

func (c *reconnectableSSHClient) Dial(ctx context.Context, n, addr string) (net.Conn, error) {
	conn, err := c.getC().Dial(n, addr)
	if err != nil {
		if rErr := c.reconnect(ctx); rErr != nil {
			return nil, err
		}
		return c.getC().Dial(n, addr)
	}
	return conn, nil
}

func (c *reconnectableSSHClient) Close() error {
	return c.getC().Close()
}

func (c *reconnectableSSHClient) KeepAlive(ctx context.Context) {
	wait := make(chan error, 1)
	go func() {
		wait <- c.getC().Wait()
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
				log.Printf("failed to keep alive of %v", c.getC().RemoteAddr())
				c.getC().Close()
				return
			}
		case <-ctx.Done():
			return
		}

		go func() {
			_, _, err := c.getC().SendRequest("keepalive@openssh.com", true, nil)
			if err != nil {
				atomic.AddUint32(&aliveErrCount, 1)
			}
		}()
	}
}

func (c *reconnectableSSHClient) getC() sshClientWrapper {
	c.mux.RLock()
	defer c.mux.RUnlock()
	return c.c
}

func (c *reconnectableSSHClient) setC(client sshClientWrapper) {
	c.mux.Lock()
	defer c.mux.Unlock()
	c.c = client
}

func (c *reconnectableSSHClient) reconnect(ctx context.Context) error {
	client, err := c.dialer.Dial()
	if err != nil {
		return err
	}
	c.getC().Close()
	c.setC(client)
	go c.KeepAlive(ctx)
	return nil
}
