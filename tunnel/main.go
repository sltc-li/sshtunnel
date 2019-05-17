package main

import (
	"encoding/json"
	"fmt"
	"io/ioutil"
	"log"
	"os"
	"sync"

	"github.com/pkg/errors"

	"github.com/li-go/sshtunnel"
)

func main() {
	if len(os.Args) < 2 {
		_, _ = fmt.Fprintln(os.Stderr, "Error: JSON config file required.")
		return
	}
	configFile := os.Args[1]

	buff, err := ioutil.ReadFile(configFile)
	if err != nil {
		_, _ = fmt.Fprintf(os.Stderr, "failed to read given config file: %s\n", configFile)
		return
	}

	var configs []Config
	err = json.Unmarshal(buff, &configs)
	if err != nil {
		_, _ = fmt.Fprintf(os.Stderr, "failed to parse config file: %s\n", configFile)
		return
	}

	logger := log.New(os.Stdout, "[sshtunnel] ", log.Flags())

	var sg sync.WaitGroup
	for _, config := range configs {
		sg.Add(1)

		go func(config Config) {
			err := forward(config, logger)
			if err != nil {
				_, _ = fmt.Fprintf(os.Stderr, "Error: %+v\n", err)
			}

			sg.Done()
		}(config)
	}
	sg.Wait()
}

func forward(config Config, logger *log.Logger) error {
	sshConfig, err := sshtunnel.NewSSHClientConfig(config.PrivateKeyFile, config.SshUserName)
	if err != nil {
		return errors.Wrap(err, "invalid ssh config")
	}

	gateway := sshtunnel.NewGateway(config.GatewayURL, sshConfig)
	tunnel := sshtunnel.NewTunnel(gateway, logger)
	if config.LocalURL == "" {
		err = tunnel.ForwardLocal(config.ForwardURL)
	} else {
		err = tunnel.Forward(config.ForwardURL, config.LocalURL)
	}
	if err != nil {
		return errors.Wrap(err, "failed to forward")
	}
	return nil
}

type Config struct {
	PrivateKeyFile string
	SshUserName    string
	GatewayURL     string
	ForwardURL     string
	LocalURL       string
}
