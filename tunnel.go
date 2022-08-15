package sshtunnel

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net"
	"strings"
)

type logger interface {
	Printf(string, ...interface{})
}

type tunnel struct {
	gateway *Gateway

	dialAddr string
	bindAddr string

	log logger
}

func NewTunnel(
	gateway *Gateway,
	tunnelStr string, // remoteAddr:port -> 127.0.0.1:port
	log logger,
) (*tunnel, error) {
	tunnelInfo := strings.Split(tunnelStr, "->")
	if len(tunnelInfo) != 2 {
		return nil, errors.New("invalid tunnel format (e.g. remoteAddr:port -> 127.0.0.1:port)")
	}
	return &tunnel{
		gateway:  gateway,
		dialAddr: strings.TrimSpace(tunnelInfo[0]),
		bindAddr: strings.TrimSpace(tunnelInfo[1]),
		log:      log,
	}, nil
}

func (t *tunnel) Forward(ctx context.Context) error {
	ctx, cancel := context.WithCancel(ctx)
	defer cancel()

	bindListener, err := closableListen(t.bindAddr)
	if err != nil {
		return fmt.Errorf("listen to bind address - %s: %w", t.bindAddr, err)
	}
	defer bindListener.Close()

	t.log.Printf("start forwarding: %s -> %s", t.dialAddr, t.bindAddr)
	defer t.log.Printf("stop forwarding: %s -> %s", t.dialAddr, t.bindAddr)

	t.startAccept(ctx, bindListener)
	return nil
}

func (t *tunnel) startAccept(ctx context.Context, bindListener *closableListener) {
	// close bind listener to stop accepting if ctx is canceled.
	go func() {
		<-ctx.Done()
		bindListener.Close()
	}()

	for {
		bindConn, err := bindListener.Accept()
		if bindListener.IsClosed() {
			break
		}
		if err != nil {
			t.log.Printf("failed to accept %s: %v", t.bindAddr, err)
			break
		}

		t.log.Printf("accepted %s -> %s", t.bindAddr, bindConn.RemoteAddr())
		go func(bindConn net.Conn) {
			defer t.log.Printf("disconnected %s -> %s", t.bindAddr, bindConn.RemoteAddr())
			defer bindConn.Close()

			dialConn, err := t.gateway.Dial(ctx, "tcp", t.dialAddr)
			if err != nil {
				t.log.Printf("failed to dial %s: %v", t.dialAddr, err)
				return
			}
			defer dialConn.Close()

			ctx, cancel := context.WithCancel(ctx)
			defer cancel()
			t.biCopy(ctx, dialConn, bindConn)
		}(bindConn)
	}
}

func (t *tunnel) biCopy(ctx context.Context, dialConn, bindConn net.Conn) {
	errCh := make(chan error)
	go copy(ctx, dialConn, bindConn, fmt.Sprintf("copy %s -> %s", t.dialAddr, t.bindAddr), errCh)
	go copy(ctx, bindConn, dialConn, fmt.Sprintf("copy %s -> %s", t.bindAddr, t.dialAddr), errCh)

	select {
	case <-ctx.Done():
	case err := <-errCh:
		if err != nil {
			t.log.Printf("failed to biCopy: %v", err)
		}
	}
}

func copy(ctx context.Context, dst io.Writer, src io.Reader, msg string, errCh chan<- error) {
	var err error
	if _, err = io.Copy(dst, src); err != nil {
		err = fmt.Errorf("%s: %v", msg, err)
	}

	select {
	case <-ctx.Done():
	case errCh <- err:
	}
}
