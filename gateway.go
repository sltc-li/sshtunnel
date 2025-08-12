package sshtunnel

import (
	"context"
	"errors"
	"fmt"
	"log"
	"net"
	"sync"
	"time"
)

func NewGateway(
	keyFiles []KeyFile,
	gatewayStr string, // user@addr:port
	gatewayProxyCommand string,
) (*Gateway, error) {
	d, err := newDialer(keyFiles, gatewayStr, gatewayProxyCommand)
	if err != nil {
		return nil, err
	}

	return &Gateway{d: d}, nil
}

type Gateway struct {
	d   dialer
	c   *sshClientWrapper
	mux sync.RWMutex
}

func (g *Gateway) Dial(ctx context.Context, n, addr string) (net.Conn, error) {
	conn, err := g.getC().Dial(n, addr)
	if err != nil {
		if errors.Is(err, errSSHClientNotInitialized) {
			if err := g.connect(ctx); err != nil {
				return nil, fmt.Errorf("connect: %w", err)
			}
		} else {
			if err := g.reconnect(ctx); err != nil {
				return nil, fmt.Errorf("reconnect: %w", err)
			}
		}

		return g.getC().Dial(n, addr)
	}

	return conn, nil
}

func (g *Gateway) Close() error {
	if g.c != nil {
		if err := g.c.Close(); err != nil {
			_ = g.d.Close()
			return err
		}
	}
	return g.d.Close()
}

func (g *Gateway) KeepAlive(ctx context.Context) {
	ticker := time.NewTicker(30 * time.Second)
	defer ticker.Stop()

	for {
		select {
		case <-ticker.C:
			c := g.getC()
			if c == nil {
				continue
			}
			if _, _, err := c.SendRequest("keepalive@openssh.com", true, nil); err != nil {
				log.Printf("ERROR: keep alive of remote(%v), local(%v)", c.RemoteAddr(), c.LocalAddr())
				if err := g.reconnect(ctx); err != nil {
					log.Printf("ERROR: reconnect: %v", err)
				}
			}
		case <-ctx.Done():
			return
		}
	}
}

func (g *Gateway) getC() *sshClientWrapper {
	g.mux.RLock()
	defer g.mux.RUnlock()
	return g.c
}

func (g *Gateway) connect(ctx context.Context) error {
	g.mux.Lock()
	defer g.mux.Unlock()

	ctx, cancel := context.WithTimeout(ctx, 30*time.Second)
	defer cancel()

	client, err := g.d.Dial(ctx)
	if err != nil {
		return err
	}

	g.c = client
	return nil
}

func (g *Gateway) reconnect(ctx context.Context) error {
	_ = g.c.Close()
	return g.connect(ctx)
}
