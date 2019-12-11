package main

import (
	"context"
	"log"
	"os"
	"os/signal"
	"sync"

	"github.com/go-yaml/yaml"

	"github.com/li-go/sshtunnel"
)

func main() {
	if len(os.Args) < 2 {
		log.Fatal("config file required")
	}
	configFile := os.Args[1]

	file, err := os.Open(configFile)
	if err != nil {
		log.Fatalf("failed to open config file: %v", err)
	}
	defer file.Close()

	var config YamlConfig
	if err := yaml.NewDecoder(file).Decode(&config); err != nil {
		file.Close()
		log.Fatalf("failed to decode config file: %v", err)
	}

	logger := log.New(os.Stdout, "[sshtunnel] ", log.Flags())

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	// handle SIGINT to stop all tunnels.
	go func() {
		defer cancel()
		signCh := make(chan os.Signal, 1)
		signal.Notify(signCh, os.Interrupt)
		logger.Printf("received %v, shutdown!", <-signCh)
	}()

	var wg sync.WaitGroup
	for _, g := range config.Gateways {
		for _, t := range g.Tunnels {
			wg.Add(1)
			go func(keyFiles []string, gatewayStr string, tunnelStr string) {
				defer wg.Done()
				tunnel, err := sshtunnel.NewTunnel(keyFiles, gatewayStr, tunnelStr, logger)
				if err != nil {
					log.Printf("failed to init tunnel - %s: %v", t, err)
					return
				}
				tunnel.Forward(ctx)
			}(config.KeyFiles, g.Server, t)
		}
	}
	wg.Wait()
}

type YamlConfig struct {
	KeyFiles []string `yaml:"key_files"`
	Gateways []struct {
		Server  string   `yaml:"server"`
		Tunnels []string `yaml:"tunnels"`
	} `yaml:"gateways"`
}
