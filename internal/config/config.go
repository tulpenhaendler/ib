package config

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strconv"
	"sync"
)

var (
	configDir  string
	configOnce sync.Once
)

// ClientConfig holds client-side configuration
type ClientConfig struct {
	ServerURL string `json:"server_url"`
	Token     string `json:"token,omitempty"`
}

// ServerConfig holds server-side configuration
type ServerConfig struct {
	Token         string `json:"token,omitempty"`
	DBPath        string `json:"db_path"`
	ListenAddr    string `json:"listen_addr"`
	RetentionDays int    `json:"retention_days"`

	// S3 configuration
	S3Endpoint  string `json:"s3_endpoint"`
	S3Bucket    string `json:"s3_bucket"`
	S3AccessKey string `json:"s3_access_key"`
	S3SecretKey string `json:"s3_secret_key"`
	S3Region    string `json:"s3_region"`
}

// Dir returns the configuration directory path
func Dir() (string, error) {
	var err error
	configOnce.Do(func() {
		configDir, err = os.UserConfigDir()
		if err != nil {
			return
		}
		configDir = filepath.Join(configDir, "ib")
		err = os.MkdirAll(configDir, 0700)
	})
	return configDir, err
}

// LoadClient loads the client configuration
func LoadClient() (*ClientConfig, error) {
	dir, err := Dir()
	if err != nil {
		return nil, err
	}

	path := filepath.Join(dir, "config.json")
	data, err := os.ReadFile(path)
	if os.IsNotExist(err) {
		return &ClientConfig{}, nil
	}
	if err != nil {
		return nil, err
	}

	var cfg ClientConfig
	if err := json.Unmarshal(data, &cfg); err != nil {
		return nil, err
	}
	return &cfg, nil
}

// SaveClient saves the client configuration
func SaveClient(cfg *ClientConfig) error {
	dir, err := Dir()
	if err != nil {
		return err
	}

	data, err := json.MarshalIndent(cfg, "", "  ")
	if err != nil {
		return err
	}

	path := filepath.Join(dir, "config.json")
	return os.WriteFile(path, data, 0600)
}

// LoadServer loads the server configuration
// Environment variables take precedence over config file
func LoadServer() (*ServerConfig, error) {
	dir, err := Dir()
	if err != nil {
		return nil, err
	}

	path := filepath.Join(dir, "server.json")
	data, err := os.ReadFile(path)

	var cfg *ServerConfig
	if os.IsNotExist(err) {
		cfg = DefaultServerConfig()
	} else if err != nil {
		return nil, err
	} else {
		cfg = &ServerConfig{}
		if err := json.Unmarshal(data, cfg); err != nil {
			return nil, err
		}
	}

	// Environment variables override config file
	if v := os.Getenv("IB_TOKEN"); v != "" {
		cfg.Token = v
	}
	if v := os.Getenv("IB_DB_PATH"); v != "" {
		cfg.DBPath = v
	}
	if v := os.Getenv("IB_LISTEN_ADDR"); v != "" {
		cfg.ListenAddr = v
	}
	if v := os.Getenv("IB_RETENTION_DAYS"); v != "" {
		if days, err := strconv.Atoi(v); err == nil {
			cfg.RetentionDays = days
		}
	}
	if v := os.Getenv("IB_S3_ENDPOINT"); v != "" {
		cfg.S3Endpoint = v
	}
	if v := os.Getenv("IB_S3_BUCKET"); v != "" {
		cfg.S3Bucket = v
	}
	if v := os.Getenv("IB_S3_ACCESS_KEY"); v != "" {
		cfg.S3AccessKey = v
	}
	if v := os.Getenv("IB_S3_SECRET_KEY"); v != "" {
		cfg.S3SecretKey = v
	}
	if v := os.Getenv("IB_S3_REGION"); v != "" {
		cfg.S3Region = v
	}

	return cfg, nil
}

// SaveServer saves the server configuration
func SaveServer(cfg *ServerConfig) error {
	dir, err := Dir()
	if err != nil {
		return err
	}

	data, err := json.MarshalIndent(cfg, "", "  ")
	if err != nil {
		return err
	}

	path := filepath.Join(dir, "server.json")
	return os.WriteFile(path, data, 0600)
}

// DefaultServerConfig returns default server configuration
func DefaultServerConfig() *ServerConfig {
	dir, _ := Dir()
	return &ServerConfig{
		DBPath:        filepath.Join(dir, "ib.db"),
		ListenAddr:    ":8080",
		RetentionDays: 90,
		S3Region:      "us-east-1",
	}
}
