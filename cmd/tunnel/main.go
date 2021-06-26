package main

import (
	"context"
	"fmt"
	"log"
	"os"
	"os/signal"
	"path/filepath"
	"sync"
	"syscall"

	"github.com/li-go/sshtunnel/syscallhelper"

	"github.com/li-go/sshtunnel"
)

func main() {
	var rLimit syscall.Rlimit
	if err := syscall.Getrlimit(syscall.RLIMIT_NOFILE, &rLimit); err != nil {
		log.Fatalf("failed to set ulimit: %v", err)
	}
	newRLimit := syscall.Rlimit{
		Cur: syscallhelper.RlimitMax(rLimit),
		Max: syscallhelper.RlimitMax(rLimit),
	}
	if err := syscall.Setrlimit(syscall.RLIMIT_NOFILE, &newRLimit); err != nil {
		log.Fatalf("failed to set ulimit: %v", err)
	}

	file, err := openConfigFile()
	if err != nil {
		log.Fatalf("failed to open config file: %v", err)
	}
	defer file.Close()

	config, err := sshtunnel.LoadConfigFile(file)
	if err != nil {
		log.Fatalf("failed to load config file: %v", err)
	}

	logger := log.New(os.Stdout, "[sshtunnel] ", log.Flags())

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, os.Kill)
	defer stop()

	var wg sync.WaitGroup
	for _, g := range config.Gateways {
		for _, t := range g.Tunnels {
			wg.Add(1)
			go func(keyFiles []sshtunnel.KeyFile, gatewayStr string, tunnelStr string) {
				defer wg.Done()
				tunnel, err := sshtunnel.NewTunnel(keyFiles, gatewayStr, tunnelStr, logger)
				if err != nil {
					log.Printf("failed to init tunnel - %s: %v", t, err)
					stop()
					return
				}
				if err := tunnel.Forward(ctx); err != nil {
					log.Printf("failed to forward tunnel - %s: %v", t, err)
					stop()
				}
			}(config.KeyFiles, g.Server, t)
		}
	}
	wg.Wait()
}

func openConfigFile() (*os.File, error) {
	if len(os.Args) > 2 {
		return nil, fmt.Errorf("too many arguments - %v", os.Args[1:])
	}

	if len(os.Args) == 2 {
		return os.Open(os.Args[1])
	}

	file, err := os.Open(".tunnel.yml")
	if err == nil {
		return file, nil
	}

	if !os.IsNotExist(err) {
		return nil, err
	}

	home, err := os.UserHomeDir()
	if err != nil {
		return nil, err
	}
	return os.Open(filepath.Join(home, ".tunnel.yml"))
}
