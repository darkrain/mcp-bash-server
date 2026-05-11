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
	LogCommands     bool     `toml:"log_commands"`
	ProcessTTL      int      `toml:"process_ttl"`
	ProcessDir      string   `toml:"process_dir"`
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
			LogCommands:     true,
			ProcessTTL:      60,
			ProcessDir:      "/var/lib/mcp-bash-server",
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
	if processTTL := os.Getenv("MCP_PROCESS_TTL"); processTTL != "" {
		var t int
		if _, err := fmt.Sscanf(processTTL, "%d", &t); err == nil {
			cfg.Bash.ProcessTTL = t
		}
	}
	if processDir := os.Getenv("MCP_PROCESS_DIR"); processDir != "" {
		cfg.Bash.ProcessDir = processDir
	}
	if level := os.Getenv("MCP_LOG_LEVEL"); level != "" {
		cfg.Log.Level = level
	}

	if err := cfg.Validate(); err != nil {
		return nil, err
	}

	return cfg, nil
}

func (c *Config) Validate() error {
	if c.Server.Port <= 0 || c.Server.Port > 65535 {
		return fmt.Errorf("invalid server port: %d", c.Server.Port)
	}
	if c.Server.BaseURL == "" {
		return fmt.Errorf("server base_url cannot be empty")
	}
	if c.Server.APIKey != "" && len(c.Server.APIKey) < 16 {
		return fmt.Errorf("api_key must be at least 16 characters long for security")
	}
	if c.Bash.Timeout < 0 {
		return fmt.Errorf("bash timeout cannot be negative")
	}
	if c.Bash.MaxOutputSize < 0 {
		return fmt.Errorf("bash max_output_size cannot be negative")
	}
	if c.Bash.ProcessTTL < 0 {
		return fmt.Errorf("bash process_ttl cannot be negative")
	}
	return nil
}

func (c *Config) ListenAddr() string {
	return fmt.Sprintf("%s:%d", c.Server.Host, c.Server.Port)
}
