package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"io/ioutil"
	"log"
	"os"
	"sync"

	"github.com/liyy7/sshtunnel"
	"github.com/pkg/errors"
)

func main() {
	var configFile string
	flag.StringVar(&configFile, "c", "", "JSON config file")
	flag.Parse()

	if configFile == "" {
		flag.Usage()
		return
	}

	buff, err := ioutil.ReadFile(configFile)
	if err != nil {
		fmt.Fprintf(os.Stderr, "failed to read given config file: %s", configFile)
		return
	}

	var configs []Config
	err = json.Unmarshal(buff, &configs)
	if err != nil {
		fmt.Fprintf(os.Stderr, "failed to parse config file: %s", configFile)
		return
	}

	logger := log.New(os.Stdout, "[sshtunnel] ", log.Flags())

	var sg sync.WaitGroup
	for _, config := range configs {
		sg.Add(1)

		go func(config Config) {
			err := forward(config, logger)
			if err != nil {
				fmt.Fprintln(os.Stderr, err)
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
