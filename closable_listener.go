package sshtunnel

import (
	"fmt"
	"net"
	"os"
	"path/filepath"
	"sync"
)

func closableListen(address string) (*closableListener, error) {
	var (
		l   net.Listener
		err error
	)
	if _, rErr := net.ResolveTCPAddr("tcp", address); rErr == nil {
		l, err = net.Listen("tcp", address)
	} else {
		// try unix socket connection
		// remove sock file is already exists
		if _, err := os.Stat(address); err == nil {
			_ = os.Remove(address)
		}
		if err := mkdirIfNeeded(address); err != nil {
			return nil, fmt.Errorf("mkdir: %w", err)
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

func mkdirIfNeeded(addr string) error {
	dir, _ := filepath.Split(addr)
	return os.MkdirAll(dir, 0700)
}
