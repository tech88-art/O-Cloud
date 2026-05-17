// Package config loads server configuration via Viper.
//
// Search order (first wins): an explicit path passed to Load → ./configs/config.yaml →
// ./config.yaml. Env var overrides use the OCEDGE_ prefix
// (e.g. OCEDGE_SERVER_PORT=9000). See configs/config.example.yaml for the
// canonical shape and field-level docs.
package config

import (
	"fmt"
	"strings"

	"github.com/spf13/viper"
)

// Defaults exported so tests and main can refer to them by name.
const (
	DefaultPort       = 8080
	DefaultLogLevel   = "info"
	envPrefix         = "OCEDGE"
	envKeyReplacement = "."
)

// Config is the deserialized YAML.
type Config struct {
	Server      ServerConfig                `mapstructure:"server"`
	Datasources map[string]DatasourceConfig `mapstructure:"datasources"`
	Mapping     map[string]string           `mapstructure:"mapping"`
	Logging     LoggingConfig               `mapstructure:"logging"`
}

// ServerConfig is the http-server section.
type ServerConfig struct {
	Port       int  `mapstructure:"port"`
	EnableCORS bool `mapstructure:"enableCORS"`
}

// DatasourceConfig is one entry in the datasources map. Fields are
// intentionally permissive so each source can read its own keys.
type DatasourceConfig struct {
	Enabled    bool   `mapstructure:"enabled"`
	Path       string `mapstructure:"path"`
	Kubeconfig string `mapstructure:"kubeconfig"`
	URL        string `mapstructure:"url"`
}

// LoggingConfig controls zap (see middleware/logging.go).
type LoggingConfig struct {
	Level string `mapstructure:"level"` // debug | info | warn | error
}

// Load reads config from configFile (if non-empty) or the search path.
// Defaults are applied first, then file, then env overrides.
func Load(configFile string) (*Config, error) {
	v := viper.New()
	applyDefaults(v)

	if configFile != "" {
		v.SetConfigFile(configFile)
	} else {
		v.SetConfigName("config")
		v.SetConfigType("yaml")
		v.AddConfigPath("./configs")
		v.AddConfigPath(".")
	}

	v.SetEnvPrefix(envPrefix)
	v.SetEnvKeyReplacer(strings.NewReplacer(envKeyReplacement, "_"))
	v.AutomaticEnv()

	// Missing config file is non-fatal — defaults + env still produce a valid Config.
	if err := v.ReadInConfig(); err != nil {
		if _, ok := err.(viper.ConfigFileNotFoundError); !ok {
			return nil, fmt.Errorf("read config: %w", err)
		}
	}

	cfg := &Config{}
	if err := v.Unmarshal(cfg); err != nil {
		return nil, fmt.Errorf("unmarshal config: %w", err)
	}
	return cfg, nil
}

func applyDefaults(v *viper.Viper) {
	v.SetDefault("server.port", DefaultPort)
	v.SetDefault("server.enableCORS", true)
	v.SetDefault("logging.level", DefaultLogLevel)
	v.SetDefault("datasources", map[string]interface{}{})
	v.SetDefault("mapping", map[string]string{})
}
