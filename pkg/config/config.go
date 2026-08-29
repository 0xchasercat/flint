package config

import (
	"encoding/json"
	"fmt"
	"net"
	"os"
	"path/filepath"
	"strconv"
	"strings"
)

// Config represents the application configuration
type Config struct {
	APIKey   string         `json:"api_key,omitempty"`
	Server   ServerConfig   `json:"server"`
	Security SecurityConfig `json:"security"`
	Libvirt  LibvirtConfig  `json:"libvirt"`
	Logging  LoggingConfig  `json:"logging"`
}

// ServerConfig represents server-specific configuration
type ServerConfig struct {
	Host         string `json:"host"`
	Port         int    `json:"port"`
	ReadTimeout  int    `json:"read_timeout"`  // seconds
	WriteTimeout int    `json:"write_timeout"` // seconds
}

// SecurityConfig represents security-related configuration
type SecurityConfig struct {
	RateLimitRequests int    `json:"rate_limit_requests"` // requests per minute
	RateLimitBurst    int    `json:"rate_limit_burst"`    // burst size
	PassphraseHash    string `json:"passphrase_hash"`     // bcrypt hash of web UI passphrase
}

// LibvirtConfig represents libvirt-related configuration
type LibvirtConfig struct {
	URI           string           `json:"uri"`
	ISOPool       string           `json:"iso_pool"`
	TemplatePool  string           `json:"template_pool"`
	ImagePoolPath string           `json:"image_pool_path"`
	SSH           LibvirtSSHConfig `json:"ssh"`
}

// LibvirtSSHConfig represents SSH-specific configuration for remote libvirt connections
type LibvirtSSHConfig struct {
	Enabled        bool   `json:"enabled"`
	Username       string `json:"username"`
	Host           string `json:"host"`
	Port           int    `json:"port"`
	KeyPath        string `json:"key_path"`
	KnownHostsPath string `json:"known_hosts_path"`
	// Password is intentionally not stored in config for security
	// Users should use SSH key authentication
}

// LoggingConfig represents logging configuration
type LoggingConfig struct {
	Level  string `json:"level"`  // DEBUG, INFO, WARN, ERROR, FATAL
	Format string `json:"format"` // json, text
}

// DefaultConfig returns the default configuration
func DefaultConfig() *Config {
	return &Config{
		Server: ServerConfig{
			Host:         "127.0.0.1",
			Port:         5550,
			ReadTimeout:  30,
			WriteTimeout: 30,
		},
		Security: SecurityConfig{
			RateLimitRequests: 100,
			RateLimitBurst:    20,
		},
		Libvirt: LibvirtConfig{
			URI:           "qemu:///system",
			ISOPool:       "isos",
			TemplatePool:  "templates",
			ImagePoolPath: "/var/lib/flint/images",
			SSH: LibvirtSSHConfig{
				Enabled:        false,
				Username:       "",
				Host:           "",
				Port:           22,
				KeyPath:        filepath.Join(os.Getenv("HOME"), ".ssh", "id_rsa"),
				KnownHostsPath: filepath.Join(os.Getenv("HOME"), ".ssh", "known_hosts"),
			},
		},
		Logging: LoggingConfig{
			Level:  "INFO",
			Format: "json",
		},
	}
}

// LoadConfig loads configuration from file and environment variables
func LoadConfig(configPath string) (*Config, error) {
	config := DefaultConfig()

	// Use default config path if not provided
	if configPath == "" {
		configPath = filepath.Join(os.Getenv("HOME"), ".flint", "config.json")
	}

	// Load from config file if it exists
	if err := loadFromFile(config, configPath); err != nil {
		return nil, fmt.Errorf("failed to load config file: %w", err)
	}

	// Override with environment variables
	loadFromEnv(config)

	return config, nil
}

// loadFromFile loads configuration from a JSON file
func loadFromFile(config *Config, path string) error {
	file, err := os.Open(path)
	if err != nil {
		if os.IsNotExist(err) {
			// Config file doesn't exist, use defaults
			return nil
		}
		return err
	}
	defer file.Close()

	decoder := json.NewDecoder(file)
	return decoder.Decode(config)
}

// loadFromEnv loads configuration from environment variables
func loadFromEnv(config *Config) {
	// Server configuration
	host := os.Getenv("FLINT_BIND_ADDRESS")
	if host == "" {
		host = os.Getenv("FLINT_SERVER_HOST")
	}
	if host != "" {
		config.Server.Host = host
	}
	port := os.Getenv("FLINT_BIND_PORT")
	if port == "" {
		port = os.Getenv("FLINT_SERVER_PORT")
	}
	if port != "" {
		if p, err := strconv.Atoi(port); err == nil {
			config.Server.Port = p
		} else {
			config.Server.Port = 0
		}
	}
	if readTimeout := os.Getenv("FLINT_SERVER_READ_TIMEOUT"); readTimeout != "" {
		if rt, err := strconv.Atoi(readTimeout); err == nil {
			config.Server.ReadTimeout = rt
		}
	}
	if writeTimeout := os.Getenv("FLINT_SERVER_WRITE_TIMEOUT"); writeTimeout != "" {
		if wt, err := strconv.Atoi(writeTimeout); err == nil {
			config.Server.WriteTimeout = wt
		}
	}

	// Security configuration
	if rateLimit := os.Getenv("FLINT_SECURITY_RATE_LIMIT"); rateLimit != "" {
		if rl, err := strconv.Atoi(rateLimit); err == nil {
			config.Security.RateLimitRequests = rl
		}
	}
	if burst := os.Getenv("FLINT_SECURITY_RATE_BURST"); burst != "" {
		if b, err := strconv.Atoi(burst); err == nil {
			config.Security.RateLimitBurst = b
		}
	}

	// Libvirt configuration
	if uri := os.Getenv("FLINT_LIBVIRT_URI"); uri != "" {
		config.Libvirt.URI = uri
	}
	if isoPool := os.Getenv("FLINT_LIBVIRT_ISO_POOL"); isoPool != "" {
		config.Libvirt.ISOPool = isoPool
	}
	if templatePool := os.Getenv("FLINT_LIBVIRT_TEMPLATE_POOL"); templatePool != "" {
		config.Libvirt.TemplatePool = templatePool
	}
	if imagePoolPath := os.Getenv("FLINT_LIBVIRT_IMAGE_POOL_PATH"); imagePoolPath != "" {
		config.Libvirt.ImagePoolPath = imagePoolPath
	}

	// SSH configuration
	if sshEnabled := os.Getenv("FLINT_LIBVIRT_SSH_ENABLED"); sshEnabled != "" {
		config.Libvirt.SSH.Enabled = sshEnabled == "true" || sshEnabled == "1"
	}
	if sshUsername := os.Getenv("FLINT_LIBVIRT_SSH_USERNAME"); sshUsername != "" {
		config.Libvirt.SSH.Username = sshUsername
	}
	if sshHost := os.Getenv("FLINT_LIBVIRT_SSH_HOST"); sshHost != "" {
		config.Libvirt.SSH.Host = sshHost
	}
	if sshPort := os.Getenv("FLINT_LIBVIRT_SSH_PORT"); sshPort != "" {
		if p, err := strconv.Atoi(sshPort); err == nil {
			config.Libvirt.SSH.Port = p
		}
	}
	if sshKeyPath := os.Getenv("FLINT_LIBVIRT_SSH_KEY_PATH"); sshKeyPath != "" {
		config.Libvirt.SSH.KeyPath = sshKeyPath
	}
	if sshKnownHostsPath := os.Getenv("FLINT_LIBVIRT_SSH_KNOWN_HOSTS_PATH"); sshKnownHostsPath != "" {
		config.Libvirt.SSH.KnownHostsPath = sshKnownHostsPath
	}

	// Logging configuration
	if level := os.Getenv("FLINT_LOG_LEVEL"); level != "" {
		config.Logging.Level = strings.ToUpper(level)
	}
	if format := os.Getenv("FLINT_LOG_FORMAT"); format != "" {
		config.Logging.Format = strings.ToLower(format)
	}
}

// SaveConfig saves the configuration to a file
func (c *Config) SaveConfig(path string) error {
	dir := filepath.Dir(path)
	if err := os.MkdirAll(dir, 0700); err != nil {
		return err
	}
	if err := os.Chmod(dir, 0700); err != nil {
		return err
	}

	file, err := os.CreateTemp(dir, ".config-*.json")
	if err != nil {
		return err
	}
	tempPath := file.Name()
	defer os.Remove(tempPath)
	if err := file.Chmod(0600); err != nil {
		file.Close()
		return err
	}

	encoder := json.NewEncoder(file)
	encoder.SetIndent("", "  ")
	if err := encoder.Encode(c); err != nil {
		file.Close()
		return err
	}
	if err := file.Sync(); err != nil {
		file.Close()
		return err
	}
	if err := file.Close(); err != nil {
		return err
	}
	if err := os.Rename(tempPath, path); err != nil {
		return err
	}
	return os.Chmod(path, 0600)
}

// GetServerAddress returns the full server address
func (c *Config) GetServerAddress() string {
	return net.JoinHostPort(c.Server.Host, strconv.Itoa(c.Server.Port))
}

// Validate validates the configuration
func (c *Config) Validate() error {
	// Validate server config
	if c.Server.Port < 1 || c.Server.Port > 65535 {
		return fmt.Errorf("invalid server port: %d", c.Server.Port)
	}
	if c.Server.ReadTimeout < 1 {
		return fmt.Errorf("read timeout must be positive")
	}
	if c.Server.WriteTimeout < 1 {
		return fmt.Errorf("write timeout must be positive")
	}

	// Validate security config
	if c.Security.RateLimitRequests < 1 {
		return fmt.Errorf("rate limit requests must be positive")
	}
	if c.Security.RateLimitBurst < 1 {
		return fmt.Errorf("rate limit burst must be positive")
	}

	// Validate libvirt config
	if c.Libvirt.URI == "" {
		return fmt.Errorf("libvirt URI cannot be empty")
	}
	if c.Libvirt.ImagePoolPath == "" {
		return fmt.Errorf("image pool path cannot be empty")
	}

	// Validate SSH config if enabled
	if c.Libvirt.SSH.Enabled {
		if c.Libvirt.SSH.Username == "" {
			return fmt.Errorf("SSH username is required when SSH is enabled")
		}
		if c.Libvirt.SSH.Host == "" {
			return fmt.Errorf("SSH host is required when SSH is enabled")
		}
		if c.Libvirt.SSH.Port < 1 || c.Libvirt.SSH.Port > 65535 {
			return fmt.Errorf("invalid SSH port: %d", c.Libvirt.SSH.Port)
		}
		if c.Libvirt.SSH.KeyPath == "" {
			return fmt.Errorf("SSH key path is required when SSH is enabled")
		}
	}

	// Validate logging config
	validLevels := map[string]bool{
		"DEBUG": true,
		"INFO":  true,
		"WARN":  true,
		"ERROR": true,
		"FATAL": true,
	}
	if !validLevels[c.Logging.Level] {
		return fmt.Errorf("invalid log level: %s", c.Logging.Level)
	}

	validFormats := map[string]bool{
		"json": true,
		"text": true,
	}
	if !validFormats[c.Logging.Format] {
		return fmt.Errorf("invalid log format: %s", c.Logging.Format)
	}

	return nil
}

// GetEffectiveLibvirtURI returns the effective libvirt URI based on SSH configuration
// If SSH is enabled, it constructs a qemu+ssh:// URI, otherwise returns the configured URI
func (c *Config) GetEffectiveLibvirtURI() string {
	if c.Libvirt.SSH.Enabled {
		// Build SSH URI: qemu+ssh://user@host:port/system
		uri := fmt.Sprintf("qemu+ssh://%s@%s", c.Libvirt.SSH.Username, c.Libvirt.SSH.Host)

		// Only add port if it's not the default SSH port (22)
		if c.Libvirt.SSH.Port != 22 {
			uri = fmt.Sprintf("%s:%d", uri, c.Libvirt.SSH.Port)
		}

		// Determine the path based on the original URI
		// Default to /system if not specified
		path := "/system"
		if strings.Contains(c.Libvirt.URI, "/session") {
			path = "/session"
		}

		uri = uri + path
		return uri
	}

	return c.Libvirt.URI
}
