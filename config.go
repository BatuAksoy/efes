package main

import "github.com/BurntSushi/toml"

// TrackerConfig holds configuration values for Tracker.
type TrackerConfig struct {
	Debug           bool   `toml:"debug"`
	ListenAddress   string `toml:"listen_address"`
	ShutdownTimeout uint32 `toml:"shutdown_timeout"`
	TempfileTooOld  uint32 `toml:"tempfile_too_old"`
}

// DatabaseConfig holds configuration values for database.
type DatabaseConfig struct {
	DSN string `toml:"dsn"`
}

// ServerConfig holds configuration values for Server.
type ServerConfig struct {
	Debug bool `toml:"debug"`
}

// Config holds configuration values for all Efes components.
type Config struct {
	Tracker  TrackerConfig
	Server   ServerConfig
	Database DatabaseConfig
}

var defaultConfig = Config{
	Tracker: TrackerConfig{
		ListenAddress:   "0.0.0.0:8001",
		ShutdownTimeout: 3000,
		TempfileTooOld:  86400000,
	},
}

// ReadConfig parses a TOML file and returns new Config.
func ReadConfig(configFile string) (*Config, error) {
	c := defaultConfig
	_, err := toml.DecodeFile(configFile, &c)
	return &c, err
}
