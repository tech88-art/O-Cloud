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
	"time"

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
	Grafana     GrafanaConfig               `mapstructure:"grafana"`
	Cache       CacheConfig                 `mapstructure:"cache"`
}

// CacheConfig is the per-resource cache eviction policy (P3-T-008).
//
// Defaults apply when a resource is missing from PerResource. Each
// resource ends up with its own pkg/cache.LRU instance constructed via
// cache.New(Options{MaxEntries, TTL}); evictions register in a shared
// cache.Catalog keyed by resource name.
//
// Prometheus instrumentation deferred (T008b / T103).
type CacheConfig struct {
	Defaults    ResourceCacheEntry            `mapstructure:"defaults"`
	PerResource map[string]ResourceCacheEntry `mapstructure:"per_resource"`
}

// ResourceCacheEntry is the per-resource (or default) cache tuning.
// TTL accepts Go duration strings like "5m" / "30s"; Viper's default
// decoder applies StringToTimeDurationHookFunc.
type ResourceCacheEntry struct {
	MaxEntries int           `mapstructure:"max_entries"`
	TTL        time.Duration `mapstructure:"ttl"`
}

// EntryFor resolves the effective entry for a given resource name,
// returning defaults when the resource has no explicit override.
func (c CacheConfig) EntryFor(resource string) ResourceCacheEntry {
	if e, ok := c.PerResource[resource]; ok {
		// Per-resource MaxEntries / TTL of 0 means "inherit default".
		if e.MaxEntries == 0 {
			e.MaxEntries = c.Defaults.MaxEntries
		}
		if e.TTL == 0 {
			e.TTL = c.Defaults.TTL
		}
		return e
	}
	return c.Defaults
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

// GrafanaConfig drives the /api/v1/grafana/url handler (P1-T-205).
//
// BaseURL is what gets prefixed onto the signed iframe URL — typically the
// host the browser reaches Grafana on (NOT the in-cluster service DNS),
// because the URL is consumed by the frontend iframe directly. Leave empty
// to fall back to api.defaultGrafanaBaseURL.
type GrafanaConfig struct {
	BaseURL string `mapstructure:"baseUrl"`
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
	// grafana.baseUrl defaults to "" — the api.GetGrafanaURL handler
	// substitutes api.defaultGrafanaBaseURL on the read side so tests can
	// build a router without populating this field.
	v.SetDefault("grafana.baseUrl", "")
	// cache.defaults (P3-T-008). Per-resource overrides land via YAML
	// `cache.per_resource.<name>.{max_entries,ttl}` — see
	// configs/config.example.yaml.
	v.SetDefault("cache.defaults.max_entries", 1024)
	v.SetDefault("cache.defaults.ttl", "5m")
	v.SetDefault("cache.per_resource", map[string]interface{}{})
}
