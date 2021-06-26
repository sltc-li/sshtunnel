package sshtunnel

type YamlConfig struct {
	KeyFiles []KeyFile `yaml:"key_files"`
	Gateways []struct {
		Server  string   `yaml:"server"`
		Tunnels []string `yaml:"tunnels"`
	} `yaml:"gateways"`
}
type KeyFile struct {
	Path       string
	Passphrase string
}

func (f *KeyFile) UnmarshalYAML(unmarshal func(interface{}) error) error {
	var raw interface{}
	unmarshal(&raw)
	switch raw := raw.(type) {
	case string:
		*f = KeyFile{
			Path: raw,
		}
	case map[interface{}]interface{}:
		path, _ := raw["path"].(string)
		passphrase, _ := raw["passphrase"].(string)
		*f = KeyFile{
			Path:       path,
			Passphrase: passphrase,
		}
	}
	return nil
}
