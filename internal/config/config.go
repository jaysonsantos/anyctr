// Package config loads anyctr's user configuration.
//
// The file is optional: anyctr works with zero configuration by detecting what is
// installed. The config exists so a team can pin a runtime, relax strictness for
// a specific flag, or point at a binary that is not on PATH.
package config

import (
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"sort"
	"strconv"
	"strings"

	"github.com/BurntSushi/toml"
)

// Config is the on-disk configuration.
type Config struct {
	// Runtime pins the runtime to use, e.g. "podman". Empty means auto-detect.
	Runtime string `toml:"runtime,omitempty"`
	// Strict fails on flags the runtime cannot honour faithfully (the default).
	Strict bool `toml:"strict"`
	// Priority is the auto-detection order. Empty means the built-in order.
	Priority []string `toml:"priority,omitempty"`
	// Binaries overrides the executable path per runtime name.
	Binaries map[string]string `toml:"binaries,omitempty"`
	// Flags overrides strictness per canonical flag: "error" or "drop". Keys may
	// be qualified by command ("run.restart") or bare ("restart").
	Flags map[string]string `toml:"flags,omitempty"`

	path string
}

// Default returns the configuration used when no file exists.
func Default() *Config {
	return &Config{Strict: true}
}

// Path reports where anyctr reads its config from. ANYCTR_CONFIG overrides
// everything; otherwise the XDG location on Unix and %APPDATA% on Windows.
func Path() string {
	if p := os.Getenv("ANYCTR_CONFIG"); p != "" {
		return p
	}
	if runtime.GOOS == "windows" {
		if dir := os.Getenv("APPDATA"); dir != "" {
			return filepath.Join(dir, "anyctr", "config.toml")
		}
	}
	if dir := os.Getenv("XDG_CONFIG_HOME"); dir != "" {
		return filepath.Join(dir, "anyctr", "config.toml")
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return ""
	}
	return filepath.Join(home, ".config", "anyctr", "config.toml")
}

// Load reads the config file. A missing file is not an error: anyctr is designed
// to be useful before anyone configures it.
func Load(path string) (*Config, error) {
	cfg := Default()
	if path == "" {
		path = Path()
	}
	cfg.path = path
	if path == "" {
		return cfg, nil
	}
	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return cfg, nil
		}
		return cfg, fmt.Errorf("reading %s: %w", path, err)
	}
	if err := toml.Unmarshal(data, cfg); err != nil {
		return cfg, fmt.Errorf("parsing %s: %w", path, err)
	}
	return cfg, nil
}

// FilePath reports the path this config was loaded from.
func (c *Config) FilePath() string { return c.path }

// Save writes the config back, creating the directory if needed.
func (c *Config) Save() error {
	if c.path == "" {
		return fmt.Errorf("no config path available (set ANYCTR_CONFIG or HOME)")
	}
	if err := os.MkdirAll(filepath.Dir(c.path), 0o755); err != nil {
		return err
	}
	f, err := os.CreateTemp(filepath.Dir(c.path), ".config.toml-*")
	if err != nil {
		return err
	}
	tmp := f.Name()
	defer os.Remove(tmp)
	if err := toml.NewEncoder(f).Encode(c); err != nil {
		f.Close()
		return err
	}
	if err := f.Close(); err != nil {
		return err
	}
	return os.Rename(tmp, c.path)
}

// Set applies a dotted key assignment, as used by `anyctr config set`.
func (c *Config) Set(key, value string) error {
	switch {
	case key == "runtime":
		c.Runtime = value
	case key == "strict":
		if value == "" {
			c.Strict = true // the default: refuse to change what was asked for
			return nil
		}
		b, err := strconv.ParseBool(value)
		if err != nil {
			return fmt.Errorf("strict must be true or false, got %q", value)
		}
		c.Strict = b
	case key == "priority":
		c.Priority = splitList(value)
	case strings.HasPrefix(key, "binaries."):
		name := strings.TrimPrefix(key, "binaries.")
		if value == "" {
			delete(c.Binaries, name)
			return nil
		}
		if c.Binaries == nil {
			c.Binaries = map[string]string{}
		}
		c.Binaries[name] = value
	case strings.HasPrefix(key, "flags."):
		if value == "" {
			delete(c.Flags, strings.TrimPrefix(key, "flags."))
			return nil
		}
		if value != "error" && value != "drop" {
			return fmt.Errorf("flag policy must be error or drop, got %q", value)
		}
		if c.Flags == nil {
			c.Flags = map[string]string{}
		}
		c.Flags[strings.TrimPrefix(key, "flags.")] = value
	default:
		return fmt.Errorf("unknown config key %q (known: runtime, strict, priority, binaries.<runtime>, flags.<flag>)", key)
	}
	return nil
}

// Get reads a dotted key back, for `anyctr config get`.
func (c *Config) Get(key string) (string, error) {
	switch {
	case key == "runtime":
		return c.Runtime, nil
	case key == "strict":
		return strconv.FormatBool(c.Strict), nil
	case key == "priority":
		return strings.Join(c.Priority, ","), nil
	case strings.HasPrefix(key, "binaries."):
		return c.Binaries[strings.TrimPrefix(key, "binaries.")], nil
	case strings.HasPrefix(key, "flags."):
		return c.Flags[strings.TrimPrefix(key, "flags.")], nil
	}
	return "", fmt.Errorf("unknown config key %q", key)
}

// Render returns the config as TOML, for `anyctr config show`.
func (c *Config) Render() (string, error) {
	var b strings.Builder
	if err := toml.NewEncoder(&b).Encode(c); err != nil {
		return "", err
	}
	return b.String(), nil
}

// FlagPolicy returns the per-flag overrides in stable order, for diagnostics.
func (c *Config) FlagPolicy() []string {
	keys := make([]string, 0, len(c.Flags))
	for k := range c.Flags {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	return keys
}

func splitList(v string) []string {
	parts := strings.FieldsFunc(v, func(r rune) bool { return r == ',' || r == ' ' })
	out := make([]string, 0, len(parts))
	for _, p := range parts {
		if p = strings.TrimSpace(p); p != "" {
			out = append(out, p)
		}
	}
	return out
}
