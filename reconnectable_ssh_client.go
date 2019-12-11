package sshtunnel

import (
	"context"
	"log"
	"net"
	"sync"
	"sync/atomic"
	"time"

	"golang.org/x/crypto/ssh"
)

func newReconnectableSSHClient(host string, config *ssh.ClientConfig) (*reconnectableSSHClient, error) {
	c, err := ssh.Dial("tcp", host, config)
	if err != nil {
		return nil, err
	}
	return &reconnectableSSHClient{host: host, config: config, c: c}, nil
}

type reconnectableSSHClient struct {
	host   string
	config *ssh.ClientConfig

	mux sync.RWMutex
	c   *ssh.Client
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
			log.Print("done")
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

func (c *reconnectableSSHClient) getC() *ssh.Client {
	c.mux.RLock()
	defer c.mux.RUnlock()
	return c.c
}

func (c *reconnectableSSHClient) setC(client *ssh.Client) {
	c.mux.Lock()
	defer c.mux.Unlock()
	c.c = client
}

func (c *reconnectableSSHClient) reconnect(ctx context.Context) error {
	client, err := ssh.Dial("tcp", c.host, c.config)
	if err != nil {
		return err
	}
	c.getC().Close()
	c.setC(client)
	go c.KeepAlive(ctx)
	return nil
}
