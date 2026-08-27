package proxy

import (
	"os"

	"gopkg.in/yaml.v3"
)
type Config struct {
    ListenAddr string `yaml:"listen_addr"`
    Strategy   string `yaml:"strategy"`
    HealthCheckIntervalSeconds int `yaml:"health_check_interval_seconds"`
    Backends   []BackendConfig `yaml:"backends"`
}

type BackendConfig struct {
    URL string `yaml:"url"`
}
func LoadConfig(path string) (*Config, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}

	var config Config
	err = yaml.Unmarshal(data, &config)
	if err != nil {
		return nil, err
	}

	return &config, nil
}