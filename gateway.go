package sshtunnel

import (
	"io/ioutil"
	"os/user"
	"strings"
	"time"

	"github.com/pkg/errors"
	"golang.org/x/crypto/ssh"
)

func NewGateway(url string, config *ssh.ClientConfig) *Gateway {
	return &Gateway{url: url, config: config}
}

type Gateway struct {
	url    string
	config *ssh.ClientConfig
}

func (g *Gateway) Dial() (*ssh.Client, error) {
	return ssh.Dial("tcp", g.url, g.config)
}

func readPrivateKeyFile(privateKeyFile string) ([]byte, error) {
	if strings.Contains(privateKeyFile, "~") {
		usr, _ := user.Current()
		privateKeyFile = strings.Replace(privateKeyFile, "~", usr.HomeDir, 1)
	}
	// use assets
	bb, err := Asset(privateKeyFile[1:])
	if err == nil {
		return bb, nil
	}
	// fallback to read file system
	return ioutil.ReadFile(privateKeyFile)
}

func NewSSHClientConfig(privateKeyFile string, userName string) (*ssh.ClientConfig, error) {
	key, err := readPrivateKeyFile(privateKeyFile)
	if err != nil {
		return nil, errors.Wrap(err, "unable to read "+privateKeyFile)
	}
	signer, err := ssh.ParsePrivateKey(key)
	if err != nil {
		return nil, errors.Wrap(err, "unable to parse private key file "+privateKeyFile)
	}
	return &ssh.ClientConfig{
		User:            userName,
		Auth:            []ssh.AuthMethod{ssh.PublicKeys(signer)},
		HostKeyCallback: ssh.InsecureIgnoreHostKey(),
		Timeout:         2 * time.Second,
	}, nil
}
