package sshtunnel

import (
	"testing"

	"github.com/go-yaml/yaml"
)

func TestKeyFileUnmarshalYAMLInvalidType(t *testing.T) {
	var kf KeyFile
	if err := yaml.Unmarshal([]byte("123"), &kf); err == nil {
		t.Fatalf("expected error")
	}
}
