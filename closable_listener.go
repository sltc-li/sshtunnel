package sshtunnel

import (
	"net"
	"sync"
)

func closableListen(network, address string) (*closableListener, error) {
	l, err := net.Listen(network, address)
	if err != nil {
		return nil, err
	}
	return &closableListener{l: l}, nil
}

type closableListener struct {
	l net.Listener

	mux    sync.RWMutex
	closed bool
}

func (l *closableListener) Accept() (net.Conn, error) {
	return l.l.Accept()
}

func (l *closableListener) Close() error {
	l.mux.Lock()
	l.closed = true
	l.mux.Unlock()
	return l.l.Close()
}

func (l *closableListener) Addr() net.Addr {
	return l.l.Addr()
}

func (l *closableListener) IsClosed() bool {
	l.mux.RLock()
	defer l.mux.RUnlock()
	return l.closed
}
