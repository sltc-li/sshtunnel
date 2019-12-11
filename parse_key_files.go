package sshtunnel

import (
	"errors"
	"fmt"
	"io/ioutil"
	"os/user"
	"strings"

	"golang.org/x/crypto/ssh"
)

func parseKeyFiles(keyFiles []string) ([]ssh.AuthMethod, error) {
	if len(keyFiles) == 0 {
		return nil, errors.New("no key file provided")
	}
	var keys []ssh.Signer
	for _, kf := range keyFiles {
		buf, err := readKeyFile(kf)
		if err != nil {
			return nil, fmt.Errorf("read key file: %w", err)
		}
		k, err := ssh.ParsePrivateKey(buf)
		if err != nil {
			return nil, fmt.Errorf("parse private key: %w", err)
		}
		keys = append(keys, k)
	}
	return []ssh.AuthMethod{ssh.PublicKeys(keys...)}, nil
}

func readKeyFile(keyFile string) ([]byte, error) {
	if strings.Contains(keyFile, "~") {
		usr, _ := user.Current()
		keyFile = strings.Replace(keyFile, "~", usr.HomeDir, 1)
	}
	// use assets
	bb, err := Asset(keyFile[1:])
	if err == nil {
		return bb, nil
	}
	// fallback to read file system
	return ioutil.ReadFile(keyFile)
}
