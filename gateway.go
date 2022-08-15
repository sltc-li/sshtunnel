package sshtunnel

import (
	"context"
	"log"
	"net"
	"sync"
	"sync/atomic"
	"time"
)

func NewGateway(
	keyFiles []KeyFile,
	gatewayStr string, // user@addr:port
	gatewayProxyCommand string,
) (*Gateway, error) {
	d, err := newDailer(keyFiles, gatewayStr, gatewayProxyCommand)
	if err != nil {
		return nil, err
	}
	return &Gateway{dialer: d}, nil
}

type Gateway struct {
	dialer

	mux sync.RWMutex
	c   sshClientWrapper
}

func (c *Gateway) Dial(ctx context.Context, n, addr string) (net.Conn, error) {
	if c.getC() == nil {
		if err := c.reconnect(ctx); err != nil {
			return nil, err
		}
		return c.getC().Dial(n, addr)
	}

	conn, err := c.getC().Dial(n, addr)
	if err != nil {
		if err := c.reconnect(ctx); err != nil {
			return nil, err
		}
		return c.getC().Dial(n, addr)
	}

	return conn, nil
}

func (c *Gateway) Close() error {
	if c.getC() != nil {
		return c.getC().Close()
	}
	return nil
}

func (c *Gateway) KeepAlive(ctx context.Context) {
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

func (c *Gateway) getC() sshClientWrapper {
	c.mux.RLock()
	defer c.mux.RUnlock()
	return c.c
}

func (c *Gateway) reconnect(ctx context.Context) error {
	if err := func() error {
		c.mux.Lock()
		defer c.mux.Unlock()

		if c.c != nil {
			c.c.Close()
		}

		ctx, cancel := context.WithTimeout(ctx, 5*time.Second)
		defer cancel()

		client, err := c.dialer.Dial(ctx)
		if err != nil {
			return err
		}

		c.c = client
		return nil
	}(); err != nil {
		return err
	}

	go c.KeepAlive(ctx)
	return nil
}
