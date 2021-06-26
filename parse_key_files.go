package sshtunnel

import (
	"errors"
	"fmt"
	"io/ioutil"
	"os/user"
	"strings"

	"golang.org/x/crypto/ssh"
)

func parseKeyFiles(keyFiles []KeyFile) ([]ssh.AuthMethod, error) {
	if len(keyFiles) == 0 {
		return nil, errors.New("no key file provided")
	}
	var keys []ssh.Signer
	for _, kf := range keyFiles {
		buf, err := readKeyFile(kf.Path)
		if err != nil {
			return nil, fmt.Errorf("read key file: %w", err)
		}
		if len(kf.Passphrase) > 0 {
			k, err := ssh.ParsePrivateKeyWithPassphrase(buf, []byte(kf.Passphrase))
			if err != nil {
				return nil, fmt.Errorf("parse private key: %w", err)
			}
			keys = append(keys, k)
		} else {
			k, err := ssh.ParsePrivateKey(buf)
			if err != nil {
				return nil, fmt.Errorf("parse private key: %w", err)
			}
			keys = append(keys, k)
		}
	}
	return []ssh.AuthMethod{ssh.PublicKeys(keys...)}, nil
}

func readKeyFile(keyFilePath string) ([]byte, error) {
	if strings.Contains(keyFilePath, "~") {
		usr, _ := user.Current()
		keyFilePath = strings.Replace(keyFilePath, "~", usr.HomeDir, 1)
	}
	// use assets
	bb, err := Asset(keyFilePath[1:])
	if err == nil {
		return bb, nil
	}
	// fallback to read file system
	return ioutil.ReadFile(keyFilePath)
}
