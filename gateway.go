package sshtunnel

import (
	"io/ioutil"
	"os/user"
	"strings"

	"github.com/pkg/errors"
	"golang.org/x/crypto/ssh"
)

type Gateway interface {
	Dial() (*ssh.Client, error)
}

func NewGateway(url string, config *ssh.ClientConfig) Gateway {
	return &gateway{url: url, config: config}
}

type gateway struct {
	url    string
	config *ssh.ClientConfig
}

func (g *gateway) Dial() (*ssh.Client, error) {
	return ssh.Dial("tcp", g.url, g.config)
}

func NewSSHClientConfig(privateKeyFile string, userName string) (*ssh.ClientConfig, error) {
	if strings.Contains(privateKeyFile, "~") {
		usr, _ := user.Current()
		privateKeyFile = strings.Replace(privateKeyFile, "~", usr.HomeDir, 1)
	}
	key, err := ioutil.ReadFile(privateKeyFile)
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
	}, nil
}
