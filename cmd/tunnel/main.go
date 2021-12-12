package main

import (
	"context"
	"fmt"
	"log"
	"os"
	"os/exec"
	"os/signal"
	"path/filepath"
	"sync"
	"syscall"

	"github.com/sevlyar/go-daemon"
	"github.com/urfave/cli/v2"

	"github.com/li-go/sshtunnel"
	"github.com/li-go/sshtunnel/syscallhelper"
)

func setupCli() {
	dCtx := func(c *cli.Context) *daemon.Context {
		return &daemon.Context{
			PidFileName: c.String("pidfile"),
			LogFileName: c.String("logfile"),
		}
	}

	app := cli.App{
		Name:    "tunnel",
		Usage:   "a tool helps to do ssh forwarding",
		Version: "0.9.0",
		Commands: cli.Commands{
			&cli.Command{
				Name:  "status",
				Usage: "show daemon process status",
				Action: func(c *cli.Context) error {
					return printDaemonStatus(dCtx(c))
				},
			},
			&cli.Command{
				Name:  "kill",
				Usage: "kill daemon process",
				Action: func(c *cli.Context) error {
					return killDaemon(dCtx(c))
				},
			},
			&cli.Command{
				Name:  "logs",
				Usage: "show daemon process logs",
				Action: func(c *cli.Context) error {
					return tailDaemonLogs(dCtx(c))
				},
			},
		},
		Flags: []cli.Flag{
			&cli.StringFlag{
				Name:    "config",
				Aliases: []string{"c"},
				Usage:   "specify a yaml config file, --config > ./.tunnel.yml > ~/.tunnel.yml",
				Value:   "./.tunnel.yml",
			},
			&cli.BoolFlag{
				Name:    "daemon",
				Aliases: []string{"d"},
				Usage:   "daemonize tunnel",
				Value:   false,
			},
			&cli.StringFlag{
				Name:  "pidfile",
				Usage: "specify pid file for daemon process",
				Value: "./.tunnel.pid",
			},
			&cli.StringFlag{
				Name:  "logfile",
				Usage: "specify log file for daemon process",
				Value: "./.tunnel.log",
			},
		},
		Action: func(c *cli.Context) error {
			if !c.Bool("daemon") {
				return start(c.String("config"))
			}

			_ = killDaemon(dCtx(c))
			p, err := dCtx(c).Reborn()
			if err != nil {
				return fmt.Errorf("reborn daemon process: %w", err)
			}
			if p != nil {
				fmt.Printf("daemon process(pid: %d) started\n", p.Pid)
				return nil
			}
			defer dCtx(c).Release()

			return start(c.String("config"))
		},
	}

	if err := app.Run(os.Args); err != nil {
		log.Fatal(err)
	}
}

func main() {
	setupCli()
}

func start(configFile string) error {
	var rLimit syscall.Rlimit
	if err := syscall.Getrlimit(syscall.RLIMIT_NOFILE, &rLimit); err != nil {
		return fmt.Errorf("get ulimit: %w", err)
	}
	newRLimit := syscall.Rlimit{
		Cur: syscallhelper.RlimitMax(rLimit),
		Max: syscallhelper.RlimitMax(rLimit),
	}
	if err := syscall.Setrlimit(syscall.RLIMIT_NOFILE, &newRLimit); err != nil {
		return fmt.Errorf("set ulimit: %v", err)
	}

	file, err := openConfigFile(configFile)
	if err != nil {
		return fmt.Errorf("open config file: %v", err)
	}
	defer file.Close()

	config, err := sshtunnel.LoadConfigFile(file)
	if err != nil {
		return fmt.Errorf("load config file: %v", err)
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

	return nil
}

func openConfigFile(configFile string) (*os.File, error) {
	var err error
	if configFile != "" {
		var file *os.File
		file, err = os.Open(configFile)
		if err == nil {
			return file, nil
		}
	}

	if err != nil && !os.IsNotExist(err) {
		return nil, err
	}

	home, err := os.UserHomeDir()
	if err != nil {
		return nil, err
	}
	return os.Open(filepath.Join(home, ".tunnel.yml"))
}

func printDaemonStatus(dCtx *daemon.Context) error {
	p, err := dCtx.Search()
	if err != nil {
		if os.IsNotExist(err) {
			fmt.Println("daemon process not running")
			return nil
		}
		return fmt.Errorf("search daemon process: %w", err)
	}
	err = p.Signal(syscall.Signal(0))
	if err == nil {
		fmt.Printf("daemon process(pid: %d) running\n", p.Pid)
	} else {
		fmt.Println("daemon process not running")
	}
	return nil
}

func killDaemon(dCtx *daemon.Context) error {
	p, err := dCtx.Search()
	if err != nil {
		return fmt.Errorf("search for daemon process: %w", err)
	}
	fmt.Printf("killing daemon process(pid: %d)\n", p.Pid)
	if err := p.Kill(); err != nil {
		return fmt.Errorf("kill daemon process(pid: %d): %w", p.Pid, err)
	}
	return os.Remove(dCtx.PidFileName)
}

func tailDaemonLogs(dCtx *daemon.Context) error {
	ctx := context.Background()
	ctx, cancel := signal.NotifyContext(ctx, os.Interrupt, os.Kill)
	defer cancel()

	cmd := exec.CommandContext(ctx, "sh", "-c", "tail -F "+dCtx.LogFileName)
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	return cmd.Run()
}
