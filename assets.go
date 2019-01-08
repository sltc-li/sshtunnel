// +build !bindata

package sshtunnel

import (
	"fmt"
)

func Asset(name string) ([]byte, error) {
	return nil, fmt.Errorf("asset %s not found", name)
}
