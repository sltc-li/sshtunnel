package sshtunnel

import (
	"context"
	"errors"
	"fmt"
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
	if err := g.c.Close(); err != nil {
		_ = g.d.Close()
		return err
	}
	return g.d.Close()
}

func (g *Gateway) KeepAlive(ctx context.Context) {
	var aliveErrCount uint32

	ticker := time.NewTicker(30 * time.Second)
	defer ticker.Stop()

	for {
		go func() {
			if g.getC() == nil {
				return
			}

			_, _, err := g.getC().SendRequest("keepalive@openssh.com", true, nil)
			if err != nil {
				atomic.StoreUint32(&aliveErrCount, 1)
			}
		}()

		select {
		case <-ticker.C:
			if atomic.LoadUint32(&aliveErrCount) == 1 {
				log.Printf("ERROR: keep alive of remote(%v), local(%v)", g.getC().RemoteAddr(), g.getC().LocalAddr())
				if err := g.reconnect(ctx); err != nil {
					log.Printf("ERROR: reconnect: %v", err)
				}

				atomic.StoreUint32(&aliveErrCount, 0)
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
