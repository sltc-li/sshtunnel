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
	"time"

	"github.com/sevlyar/go-daemon"
	"github.com/urfave/cli/v2"

	"github.com/adrg/xdg"

	"github.com/sltc-li/sshtunnel"
	"github.com/sltc-li/sshtunnel/syscallhelper"
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
			&cli.Command{
				Name:  "reload",
				Usage: "reload config",
				Action: func(c *cli.Context) error {
					return reloadConfig(dCtx(c))
				},
			},
		},
		Flags: []cli.Flag{
			&cli.StringFlag{
				Name:    "config",
				Aliases: []string{"c"},
				Usage:   "specify a yaml config file (try -c > ./.tunnel.yml > ~/.tunnel.yml in order)",
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

func init() {
	l := log.Default()
	l.SetPrefix("[sshtunnel] ")
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

	starter := newStarter()

	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, os.Interrupt, os.Kill, syscall.SIGHUP)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	loadErrCh := make(chan error)
	if err := starter.load(ctx, configFile, loadErrCh); err != nil {
		return fmt.Errorf("load config: %w", err)
	}

	stop := func() {
		cancel()
		time.Sleep(time.Second)
	}

	for {
		select {
		case err := <-loadErrCh:
			stop()
			return fmt.Errorf("load config: %w", err)
		case sig := <-sigCh:
			switch sig {
			case os.Interrupt, os.Kill:
				stop()
				return nil
			case syscall.SIGHUP:
				// reload config
				if err := starter.load(ctx, configFile, loadErrCh); err != nil {
					stop()
					return fmt.Errorf("reload config: %w", err)
				}
			}
		}
	}
}

type Starter struct {
	config *sshtunnel.YAMLConfig
	stop   func()
}

func newStarter() *Starter {
	return &Starter{}
}

func (s *Starter) load(ctx context.Context, configFile string, errCh chan<- error) error {
	config, err := loadConfig(configFile)
	if err != nil {
		return err
	}

	if s.config.Equals(config) {
		log.Print("config not change")
		return nil
	}

	s.config = config

	if s.stop != nil {
		s.stop()
		time.Sleep(time.Second)
	}

	ctx, s.stop = context.WithCancel(ctx)
	go s.startTunnels(ctx, errCh)
	return nil
}

func (s *Starter) startTunnels(ctx context.Context, errCh chan<- error) {
	if s.config == nil {
		log.Print("not initialized")
		return
	}

	var wg sync.WaitGroup
	for _, g := range s.config.Gateways {
		gateway, err := sshtunnel.NewGateway(s.config.KeyFiles, g.Server, g.ProxyCommand)
		if err != nil {
			log.Printf("ERROR: init gateway %s: %v", g.Server, err)
			return
		}

		go gateway.KeepAlive(ctx)

		for _, t := range g.Tunnels {
			wg.Add(1)
			go func(tunnelStr string) {
				defer wg.Done()
				tunnel, err := sshtunnel.NewTunnel(gateway, tunnelStr)
				if err != nil {
					errCh <- fmt.Errorf("init tunnel - %s: %w", tunnelStr, err)
					return
				}
				if err := tunnel.Forward(ctx); err != nil {
					errCh <- fmt.Errorf("forward tunnel - %s: %w", tunnelStr, err)
				}
			}(t)
		}
	}
	wg.Wait()
}

func loadConfig(configFile string) (*sshtunnel.YAMLConfig, error) {
	file, err := openConfigFile(configFile)
	if err != nil {
		return nil, fmt.Errorf("open config file: %v", err)
	}
	defer file.Close()

	config, err := sshtunnel.LoadConfigFile(file)
	if err != nil {
		return nil, fmt.Errorf("load config file: %v", err)
	}

	return config, nil
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

	cfp, err := xdg.SearchConfigFile("sshtunnel/.tunnel.yml")
	if err != nil {
		home, err := os.UserHomeDir()
		if err != nil {
			return nil, err
		}
		return os.Open(filepath.Join(home, ".tunnel.yml"))
	}

	return os.Open(cfp)
}

func printDaemonStatus(dCtx *daemon.Context) error {
	process, running, err := daemonRunning(dCtx)
	if err != nil {
		return err
	}
	if !running {
		log.Print("daemon process not running")
		return nil
	}
	log.Printf("daemon process(pid: %d) running\n", process.Pid)
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

func reloadConfig(dCtx *daemon.Context) error {
	p, running, err := daemonRunning(dCtx)
	if err != nil {
		return err
	}
	if !running {
		fmt.Println("daemon process not running")
		return nil
	}
	err = p.Signal(syscall.SIGHUP)
	if err == nil {
		fmt.Println("config reloaded")
	}
	return err
}

func daemonRunning(dCtx *daemon.Context) (process *os.Process, running bool, err error) {
	p, err := dCtx.Search()
	if err != nil {
		if os.IsNotExist(err) {
			return nil, false, nil
		}
		return nil, false, fmt.Errorf("search daemon process: %w", err)
	}
	err = p.Signal(syscall.Signal(0))
	if err != nil {
		return p, false, nil
	}
	return p, true, nil
}
