// Package config loads relay configuration from an optional JSON file with
// environment-variable overrides. No third-party dependencies.
package config

import (
	"encoding/json"
	"fmt"
	"os"
	"strings"
)

// TV is a single controllable television.
type TV struct {
	Name string `json:"name"`
	Addr string `json:"addr"` // host or host:port; :5555 is assumed if no port
}

// Config is the full relay configuration.
type Config struct {
	HTTPAddr string `json:"httpAddr"` // listen address for the REST API, e.g. ":8080" ("" disables)
	TCPAddr  string `json:"tcpAddr"`  // listen address for the line protocol, e.g. ":8700" ("" disables)
	KeyPath  string `json:"keyPath"`  // path to the persisted ADB RSA key
	KeyName  string `json:"keyName"`  // label shown in the TV's "Allow debugging" dialog
	TVs      []TV   `json:"tvs"`
}

// Default returns a Config populated with sensible defaults.
func Default() Config {
	return Config{
		HTTPAddr: ":8080",
		TCPAddr:  ":8700",
		KeyPath:  "/data/adbkey",
		KeyName:  "android-tv-relay",
	}
}

// Load builds a Config: defaults, then the JSON file at path (if non-empty and
// present), then environment overrides. It validates the result.
func Load(path string) (Config, error) {
	cfg := Default()

	if path != "" {
		data, err := os.ReadFile(path)
		if err != nil {
			return cfg, fmt.Errorf("read config %s: %w", path, err)
		}
		if err := json.Unmarshal(data, &cfg); err != nil {
			return cfg, fmt.Errorf("parse config %s: %w", path, err)
		}
	}

	applyEnv(&cfg)
	normalize(&cfg)

	if len(cfg.TVs) == 0 {
		return cfg, fmt.Errorf("no TVs configured (set tvs in the config file, or TVS / TV_ADDR env)")
	}
	if cfg.HTTPAddr == "" && cfg.TCPAddr == "" {
		return cfg, fmt.Errorf("both HTTP and TCP listeners are disabled")
	}
	seen := map[string]bool{}
	for _, tv := range cfg.TVs {
		if tv.Name == "" || tv.Addr == "" {
			return cfg, fmt.Errorf("TV entry missing name or addr: %+v", tv)
		}
		if seen[tv.Name] {
			return cfg, fmt.Errorf("duplicate TV name %q", tv.Name)
		}
		seen[tv.Name] = true
	}
	return cfg, nil
}

func applyEnv(cfg *Config) {
	if v, ok := os.LookupEnv("HTTP_ADDR"); ok {
		cfg.HTTPAddr = v
	}
	if v, ok := os.LookupEnv("TCP_ADDR"); ok {
		cfg.TCPAddr = v
	}
	if v, ok := os.LookupEnv("ADB_KEY_PATH"); ok {
		cfg.KeyPath = v
	}
	if v, ok := os.LookupEnv("ADB_KEY_NAME"); ok {
		cfg.KeyName = v
	}
	// TVS="living=192.168.1.50,hall=192.168.1.51:5555"
	if v, ok := os.LookupEnv("TVS"); ok && strings.TrimSpace(v) != "" {
		var tvs []TV
		for _, part := range strings.Split(v, ",") {
			part = strings.TrimSpace(part)
			if part == "" {
				continue
			}
			name, addr, found := strings.Cut(part, "=")
			if !found {
				name, addr = "tv", part
			}
			tvs = append(tvs, TV{Name: strings.TrimSpace(name), Addr: strings.TrimSpace(addr)})
		}
		cfg.TVs = tvs
	}
	// Single-TV convenience.
	if addr, ok := os.LookupEnv("TV_ADDR"); ok && strings.TrimSpace(addr) != "" {
		name := os.Getenv("TV_NAME")
		if name == "" {
			name = "tv"
		}
		cfg.TVs = []TV{{Name: name, Addr: strings.TrimSpace(addr)}}
	}
}

func normalize(cfg *Config) {
	for i := range cfg.TVs {
		cfg.TVs[i].Addr = withPort(cfg.TVs[i].Addr)
	}
}

// withPort appends the default ADB port if the address has none.
func withPort(addr string) string {
	if addr == "" {
		return addr
	}
	// IPv6 literals use [..]:port; a bare "host" or "ipv4" has no colon.
	if strings.HasPrefix(addr, "[") {
		if strings.Contains(addr, "]:") {
			return addr
		}
		return addr + ":5555"
	}
	if strings.Contains(addr, ":") {
		return addr
	}
	return addr + ":5555"
}
