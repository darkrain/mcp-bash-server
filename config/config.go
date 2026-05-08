package config

import (
	"fmt"
	"os"

	"github.com/BurntSushi/toml"
)

type Config struct {
	Server ServerConfig `toml:"server"`
	Bash   BashConfig   `toml:"bash"`
	Log    LogConfig    `toml:"log"`
}

type ServerConfig struct {
	Host    string `toml:"host"`
	Port    int    `toml:"port"`
	APIKey  string `toml:"api_key"`
	BaseURL string `toml:"base_url"`
}

type BashConfig struct {
	AllowedCommands []string `toml:"allowed_commands"`
	Timeout         int      `toml:"timeout"`
	MaxOutputSize   int      `toml:"max_output_size"`
}

type LogConfig struct {
	Level  string `toml:"level"`
	Format string `toml:"format"`
	Output string `toml:"output"`
}

func DefaultConfig() *Config {
	return &Config{
		Server: ServerConfig{
			Host:    "0.0.0.0",
			Port:    8080,
			APIKey:  "",
			BaseURL: "/mcp",
		},
		Bash: BashConfig{
			AllowedCommands: []string{},
			Timeout:         30,
			MaxOutputSize:   1048576,
		},
		Log: LogConfig{
			Level:  "info",
			Format: "json",
			Output: "stderr",
		},
	}
}

func Load(path string) (*Config, error) {
	cfg := DefaultConfig()

	if path != "" {
		if _, err := os.Stat(path); err == nil {
			if _, err := toml.DecodeFile(path, cfg); err != nil {
				return nil, fmt.Errorf("failed to decode config file: %w", err)
			}
		}
	}

	if host := os.Getenv("MCP_HOST"); host != "" {
		cfg.Server.Host = host
	}
	if port := os.Getenv("MCP_PORT"); port != "" {
		var p int
		if _, err := fmt.Sscanf(port, "%d", &p); err == nil {
			cfg.Server.Port = p
		}
	}
	if apiKey := os.Getenv("MCP_API_KEY"); apiKey != "" {
		cfg.Server.APIKey = apiKey
	}
	if baseURL := os.Getenv("MCP_BASE_URL"); baseURL != "" {
		cfg.Server.BaseURL = baseURL
	}
	if timeout := os.Getenv("MCP_BASH_TIMEOUT"); timeout != "" {
		var t int
		if _, err := fmt.Sscanf(timeout, "%d", &t); err == nil {
			cfg.Bash.Timeout = t
		}
	}
	if level := os.Getenv("MCP_LOG_LEVEL"); level != "" {
		cfg.Log.Level = level
	}

	return cfg, nil
}

func (c *Config) ListenAddr() string {
	return fmt.Sprintf("%s:%d", c.Server.Host, c.Server.Port)
}
