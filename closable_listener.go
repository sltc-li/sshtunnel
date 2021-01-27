package sshtunnel

import (
	"net"
	"os"
	"regexp"
	"sync"
)

var (
	tcpAddressPattern = regexp.MustCompile(`(.+\.)+\w+:\d+`)
)

func closableListen(address string) (*closableListener, error) {
	var (
		l   net.Listener
		err error
	)
	if tcpAddressPattern.MatchString(address) {
		l, err = net.Listen("tcp", address)
	} else {
		// try unix socket connection
		// remove sock file is already exists
		if _, err := os.Stat(address); err == nil {
			_ = os.Remove(address)
		}
		l, err = net.Listen("unix", address)
	}
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
