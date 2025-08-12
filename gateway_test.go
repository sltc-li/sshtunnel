package sshtunnel

import (
	"context"
	"testing"
)

func TestGatewayKeepAliveNilConn(t *testing.T) {
	g := &Gateway{}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	g.KeepAlive(ctx)
}
