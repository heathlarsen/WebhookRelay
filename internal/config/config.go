package config

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path"
	"strings"
	"time"
)

type Config struct {
	Server ServerConfig  `json:"server"`
	Relays []RelayConfig `json:"relays"`
}

type ServerConfig struct {
	ListenAddr       string `json:"listen_addr"`
	BasePath         string `json:"base_path,omitempty"`
	ForwardTimeoutMS int    `json:"forward_timeout_ms,omitempty"`
	Concurrency      int    `json:"concurrency,omitempty"`
}

func (s ServerConfig) ForwardTimeout() time.Duration {
	ms := s.ForwardTimeoutMS
	if ms <= 0 {
		ms = 10_000
	}
	return time.Duration(ms) * time.Millisecond
}

type RelayConfig struct {
	Name         string              `json:"name,omitempty"`
	ListenPath   string              `json:"listen_path,omitempty"`
	Methods      []string            `json:"methods,omitempty"`
	Destinations []DestinationConfig `json:"destinations"`
}

type DestinationConfig struct {
	URL         string            `json:"url"`
	Method      string            `json:"method,omitempty"`
	Headers     map[string]string `json:"headers,omitempty"`
	Description string            `json:"description,omitempty"`
}

func Load(configPath string) (Config, error) {
	b, err := os.ReadFile(configPath)
	if err != nil {
		return Config{}, fmt.Errorf("read config: %w", err)
	}

	dec := json.NewDecoder(bytes.NewReader(b))
	dec.DisallowUnknownFields()

	var cfg Config
	if err := dec.Decode(&cfg); err != nil {
		return Config{}, fmt.Errorf("parse config json: %w", err)
	}
	// Ensure no trailing tokens.
	if err := dec.Decode(&struct{}{}); err != io.EOF {
		return Config{}, fmt.Errorf("parse config json: extra data after first JSON object")
	}

	if err := validateAndDefault(&cfg); err != nil {
		return Config{}, err
	}

	return cfg, nil
}

func validateAndDefault(cfg *Config) error {
	var problems []string

	if strings.TrimSpace(cfg.Server.ListenAddr) == "" {
		problems = append(problems, "server.listen_addr is required")
	}

	if cfg.Server.Concurrency <= 0 {
		cfg.Server.Concurrency = 50
	}
	if cfg.Server.ForwardTimeoutMS <= 0 {
		cfg.Server.ForwardTimeoutMS = 10_000
	}

	cfg.Server.BasePath = normalizeBasePath(cfg.Server.BasePath)

	if len(cfg.Relays) == 0 {
		problems = append(problems, "relays must be a non-empty array")
	}

	for i := range cfg.Relays {
		r := &cfg.Relays[i]

		if len(r.Methods) == 0 {
			r.Methods = []string{"POST"}
		}
		for mi := range r.Methods {
			r.Methods[mi] = strings.ToUpper(strings.TrimSpace(r.Methods[mi]))
			if r.Methods[mi] == "" {
				problems = append(problems, fmt.Sprintf("relays[%d].methods contains an empty method", i))
			}
		}

		if len(r.Destinations) == 0 {
			problems = append(problems, fmt.Sprintf("relays[%d].destinations must be non-empty", i))
			continue
		}
		for di := range r.Destinations {
			d := &r.Destinations[di]
			if strings.TrimSpace(d.URL) == "" {
				problems = append(problems, fmt.Sprintf("relays[%d].destinations[%d].url is required", i, di))
			}
			d.Method = strings.ToUpper(strings.TrimSpace(d.Method))
		}

		if r.ListenPath != "" && !strings.HasPrefix(r.ListenPath, "/") {
			// Keep it simple: require leading slash if user sets it.
			problems = append(problems, fmt.Sprintf("relays[%d].listen_path must start with '/' (got %q)", i, r.ListenPath))
		}
	}

	if len(problems) > 0 {
		return errors.New(strings.Join(problems, "; "))
	}
	return nil
}

func normalizeBasePath(p string) string {
	p = strings.TrimSpace(p)
	if p == "" || p == "/" {
		return ""
	}
	if !strings.HasPrefix(p, "/") {
		p = "/" + p
	}
	p = path.Clean(p)
	if p == "/" {
		return ""
	}
	return strings.TrimRight(p, "/")
}
