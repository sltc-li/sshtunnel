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

var (
	errSSHClientNotInitialized = errors.New("ssh client not initialized")
)

type sshClientWrapper struct {
	*ssh.Client
	cmd *exec.Cmd
}

func (c *sshClientWrapper) Dial(n, addr string) (net.Conn, error) {
	if c == nil {
		return nil, errSSHClientNotInitialized
	}
	return c.Client.Dial(n, addr)
}

func (c *sshClientWrapper) Close() error {
	err := c.Client.Close()
	if c.cmd != nil {
		_ = syscall.Kill(-c.cmd.Process.Pid, syscall.SIGKILL)
	}
	return err
}

type dialer interface {
	Dial(ctx context.Context) (*sshClientWrapper, error)
	Close() error
}

func newDailer(
	keyFiles []KeyFile,
	gatewayStr string, // user@addr:port
	gatewayProxyCommand string,
) (dialer, error) {
	auth, cleanup, err := parseKeyFiles(keyFiles)
	if err != nil {
		cleanup()
		return nil, fmt.Errorf("parse key files: %w", err)
	}
	gatewayInfo := strings.Split(gatewayStr, "@")
	if len(gatewayInfo) != 2 {
		cleanup()
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
		return newTCPDialer(gatewayHost, config, cleanup), nil
	}
	return newProxyDialer(gatewayHost, config, cleanup, gatewayProxyCommand), nil
}

type tcpDialer struct {
	host          string
	config        *ssh.ClientConfig
	configCleanup func()
}

func newTCPDialer(host string, config *ssh.ClientConfig, configCleanup func()) *tcpDialer {
	return &tcpDialer{
		host:          host,
		config:        config,
		configCleanup: configCleanup,
	}
}

func (d *tcpDialer) Dial(ctx context.Context) (*sshClientWrapper, error) {
	client, err := ssh.Dial("tcp", d.host, d.config)
	if err != nil {
		return nil, fmt.Errorf("dial gateway %s: %w", d.host, err)
	}

	return &sshClientWrapper{Client: client}, nil
}

func (d *tcpDialer) Close() error {
	d.configCleanup()
	return nil
}

type proxyDialer struct {
	host          string
	config        *ssh.ClientConfig
	configCleanup func()
	proxyCommand  string
}

func newProxyDialer(host string, config *ssh.ClientConfig, configCleanup func(), proxyCommand string) *proxyDialer {
	addr, port, _ := net.SplitHostPort(host)
	proxyCommand = strings.Replace(proxyCommand, "%h", addr, -1)
	proxyCommand = strings.Replace(proxyCommand, "%p", port, -1)
	return &proxyDialer{
		host:          host,
		config:        config,
		configCleanup: configCleanup,
		proxyCommand:  proxyCommand,
	}
}

func (d *proxyDialer) Dial(ctx context.Context) (*sshClientWrapper, error) {
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
		return &sshClientWrapper{
			cmd:    cmd,
			Client: client,
		}, nil
	case <-ctx.Done():
		if cmd.Process != nil {
			_ = syscall.Kill(-cmd.Process.Pid, syscall.SIGKILL)
		}
		return nil, ctx.Err()
	}
}

func (d *proxyDialer) Close() error {
	d.configCleanup()
	return nil
}
