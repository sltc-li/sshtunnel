package main

import (
	"context"
	"flag"
	"fmt"
	"log"
	"os"
	"os/signal"
	"path/filepath"
	"sync"
	"syscall"

	"github.com/sevlyar/go-daemon"

	"github.com/li-go/sshtunnel"
	"github.com/li-go/sshtunnel/syscallhelper"
)

var (
	daemonize bool
	kill      bool
)

func init() {
	flag.BoolVar(&daemonize, "d", false, "Daemonize tunnel")
	flag.BoolVar(&kill, "kill", false, "Kill tunnel daemon process")
	flag.Parse()
}

func main() {
	dCtx := daemon.Context{
		PidFileName: ".tunnel.pid",
		LogFileName: ".tunnel.log",
	}
	if kill {
		if err := killDaemon(dCtx); err != nil {
			log.Fatal(err)
		}
		return
	}

	if !daemonize {
		_main()
		return
	}

	_ = killDaemon(dCtx)
	p, err := dCtx.Reborn()
	if err != nil {
		log.Fatal(err)
	}
	if p != nil {
		fmt.Printf("daemon process started - %d\n", p.Pid)
		return
	}
	defer dCtx.Release()

	_main()
}

func _main() {
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
			go func(keyFiles []sshtunnel.KeyFile, gatewayStr string, gatewayProxyCommand string, tunnelStr string) {
				defer wg.Done()
				tunnel, err := sshtunnel.NewTunnel(keyFiles, gatewayStr, gatewayProxyCommand, tunnelStr, logger)
				if err != nil {
					log.Printf("failed to init tunnel - %s: %v", t, err)
					stop()
					return
				}
				if err := tunnel.Forward(ctx); err != nil {
					log.Printf("failed to forward tunnel - %s: %v", t, err)
					stop()
				}
			}(config.KeyFiles, g.Server, g.ProxyCommand, t)
		}
	}
	wg.Wait()
}

func openConfigFile() (*os.File, error) {
	if len(flag.Args()) > 2 {
		return nil, fmt.Errorf("too many arguments - %v", flag.Args()[1:])
	}

	if flag.NArg() == 2 {
		return os.Open(flag.Arg(1))
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

func killDaemon(dCtx daemon.Context) error {
	p, err := dCtx.Search()
	if err != nil {
		return fmt.Errorf("search for daemon process: %w", err)
	}
	fmt.Printf("killing daemon process - %d\n", p.Pid)
	if err := p.Kill(); err != nil {
		return fmt.Errorf("kill daemon process: %w", err)
	}
	return os.Remove(dCtx.PidFileName)
}
