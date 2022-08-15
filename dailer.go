package sshtunnel

import (
	"context"
	"errors"
	"fmt"
	"net"
	"os"
	"os/exec"
	"strings"
	"syscall"
	"time"

	"golang.org/x/crypto/ssh"
)

type sshClientWrapper interface {
	ssh.Conn
	Dial(n, addr string) (net.Conn, error)
}

type dialer interface {
	Dial(ctx context.Context) (sshClientWrapper, error)
}

func newDailer(
	keyFiles []KeyFile,
	gatewayStr string, // user@addr:port
	gatewayProxyCommand string,
) (dialer, error) {
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
	config := &ssh.ClientConfig{
		User:            gatewayUser,
		Auth:            auth,
		HostKeyCallback: ssh.InsecureIgnoreHostKey(),
		Timeout:         2 * time.Second,
	}
	if gatewayProxyCommand == "" {
		return newTCPDialer(gatewayHost, config), nil
	}
	return newProxyDialer(gatewayHost, config, gatewayProxyCommand), nil
}

type tcpDialer struct {
	host   string
	config *ssh.ClientConfig

	client *closableSSHClient
}

func newTCPDialer(host string, config *ssh.ClientConfig) *tcpDialer {
	return &tcpDialer{
		host:   host,
		config: config,
	}
}

func (d *tcpDialer) Dial(ctx context.Context) (sshClientWrapper, error) {
	if d.client != nil && d.client.Client != nil {
		return d.client, nil
	}

	client, err := ssh.Dial("tcp", d.host, d.config)
	if err != nil {
		return nil, fmt.Errorf("dial gateway %s: %w", d.host, err)
	}

	d.client = &closableSSHClient{Client: client}
	return d.client, nil
}

type closableSSHClient struct {
	*ssh.Client
}

func (c *closableSSHClient) Close() error {
	err := c.Client.Close()
	c.Client = nil
	return err
}

type proxyDialer struct {
	host         string
	config       *ssh.ClientConfig
	proxyCommand string

	client *proxySSHClient
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

func (d *proxyDialer) Dial(ctx context.Context) (sshClientWrapper, error) {
	if d.client != nil && d.client.Client != nil {
		return d.client, nil
	}

	clientConn, proxyConn := net.Pipe()
	cmd := exec.Command("bash", "-c", d.proxyCommand)
	cmd.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}
	cmd.Stdin = proxyConn
	cmd.Stdout = proxyConn
	cmd.Stderr = os.Stderr

	errCh := make(chan error)
	go func() {
		if err := cmd.Run(); err != nil {
			errCh <- fmt.Errorf("start proxy command: %w", err)
		}
	}()

	clientCh := make(chan *ssh.Client)
	go func() {
		conn, incomingChannels, incomingRequests, err := ssh.NewClientConn(clientConn, d.host, d.config)
		if err != nil {
			errCh <- fmt.Errorf("dial gateway %s via proxy: %w", d.host, err)
			return
		}

		clientCh <- ssh.NewClient(conn, incomingChannels, incomingRequests)
	}()

	select {
	case err := <-errCh:
		return nil, err
	case client := <-clientCh:
		d.client = &proxySSHClient{
			cmd:    cmd,
			Client: client,
		}
		return d.client, nil
	case <-ctx.Done():
		return nil, ctx.Err()
	}
}

type proxySSHClient struct {
	cmd *exec.Cmd
	*ssh.Client
}

func (c *proxySSHClient) Close() error {
	if c.Client == nil {
		return nil
	}

	err := c.Client.Close()
	c.Client = nil
	_ = syscall.Kill(-c.cmd.Process.Pid, syscall.SIGKILL)
	return err
}
