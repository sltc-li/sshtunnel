package sshtunnel

import (
	"fmt"
	"io/ioutil"
	"net"
	"os"
	"os/user"
	"strings"

	"golang.org/x/crypto/ssh"
	"golang.org/x/crypto/ssh/agent"
)

func parseKeyFiles(keyFiles []KeyFile) ([]ssh.AuthMethod, func(), error) {
	var keys []ssh.Signer
	var cleanup = func() {}

	if sock, ok := os.LookupEnv("SSH_AUTH_SOCK"); ok {
		conn, err := net.Dial("unix", sock)
		if err != nil {
			return nil, func() {}, fmt.Errorf("dial ssh auth sock: %w", err)
		}

		cleanup = func() {
			_ = conn.Close()
		}

		signers, err := agent.NewClient(conn).Signers()
		if err != nil {
			return nil, cleanup, fmt.Errorf("get signers from ssh agent: %w", err)
		}
		keys = append(keys, signers...)
	}

	for _, kf := range keyFiles {
		buf, err := readKeyFile(kf.Path)
		if err != nil {
			return nil, cleanup, fmt.Errorf("read key file: %w", err)
		}
		if len(kf.Passphrase) > 0 {
			k, err := ssh.ParsePrivateKeyWithPassphrase(buf, []byte(kf.Passphrase))
			if err != nil {
				cleanup()
				return nil, cleanup, fmt.Errorf("parse private key: %w", err)
			}
			keys = append(keys, k)
		} else {
			k, err := ssh.ParsePrivateKey(buf)
			if err != nil {
				cleanup()
				return nil, cleanup, fmt.Errorf("parse private key: %w", err)
			}
			keys = append(keys, k)
		}
	}
	return []ssh.AuthMethod{ssh.PublicKeys(keys...)}, cleanup, nil
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
