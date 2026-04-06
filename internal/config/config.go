// Package config handles loading and merging of prism configuration.
package config

import (
	"errors"
	"fmt"
	"os"
	"time"

	"gopkg.in/yaml.v3"
)

// Config holds prism settings from .prism.yml and CLI flags.
// Pointer fields (MaxRetries) use nil to distinguish "not set" from
// an explicit zero value, which is meaningful (e.g. 0 retries = disabled).
type Config struct {
	Roles            []string `yaml:"roles"`
	Model            string   `yaml:"model"`
	Format           string   `yaml:"format"`
	AgentTimeout     string   `yaml:"agent_timeout"`
	MaxRetries       *int     `yaml:"max_retries"`
	MaxBudgetUSD     float64  `yaml:"max_budget_usd"`
	DiffContextLines *int     `yaml:"diff_context_lines"` // nil = use default (1), -1 = keep all
	Debate           bool     `yaml:"debate"`
	NoCompress       bool     `yaml:"no_compress"`
	StripPatterns    []string `yaml:"strip_patterns"`
	DiffWarnBytes    int      `yaml:"diff_warn_bytes"`
	DiffChunkBytes   int      `yaml:"diff_chunk_bytes"`
}

// IntPtr returns a pointer to the given int. Convenience for config construction.
func IntPtr(n int) *int { return &n }

// Default returns a Config with sensible defaults.
func Default() Config {
	return Config{
		Format:         "html",
		AgentTimeout:   "5m",
		MaxRetries:     IntPtr(1),
		DiffWarnBytes:  153600, // 150 KB
		DiffChunkBytes: 307200, // 300 KB
	}
}

// Load reads a YAML config file. Returns a zero-value Config if the file
// does not exist, allowing repos without .prism.yml to work unchanged.
func Load(path string) (Config, error) {
	data, err := os.ReadFile(path) // #nosec G304 -- path is from CLI flag with default ".prism.yml"
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return Config{}, nil
		}
		return Config{}, err
	}

	var cfg Config
	if err := yaml.Unmarshal(data, &cfg); err != nil {
		return Config{}, err
	}

	// Validate duration strings early so typos are caught at load time.
	if cfg.AgentTimeout != "" {
		if _, err := time.ParseDuration(cfg.AgentTimeout); err != nil {
			return Config{}, fmt.Errorf("invalid agent_timeout %q: %w", cfg.AgentTimeout, err)
		}
	}

	return cfg, nil
}

// Merge combines configs with precedence: cli > file > defaults.
// Zero-value fields are treated as "not set" and skipped.
func Merge(def, file, cli *Config) Config {
	out := *def

	// File overrides defaults.
	mergeInto(&out, file)
	// CLI overrides file.
	mergeInto(&out, cli)

	return out
}

func mergeInto(dst, src *Config) {
	if len(src.Roles) > 0 {
		dst.Roles = src.Roles
	}
	if src.Model != "" {
		dst.Model = src.Model
	}
	if src.Format != "" {
		dst.Format = src.Format
	}
	if src.AgentTimeout != "" {
		dst.AgentTimeout = src.AgentTimeout
	}
	if src.MaxRetries != nil {
		dst.MaxRetries = src.MaxRetries
	}
	if src.MaxBudgetUSD > 0 {
		dst.MaxBudgetUSD = src.MaxBudgetUSD
	}
	if src.DiffContextLines != nil {
		dst.DiffContextLines = src.DiffContextLines
	}
	if src.Debate {
		dst.Debate = true
	}
	if src.NoCompress {
		dst.NoCompress = true
	}
	if len(src.StripPatterns) > 0 {
		dst.StripPatterns = src.StripPatterns
	}
	if src.DiffWarnBytes > 0 {
		dst.DiffWarnBytes = src.DiffWarnBytes
	}
	if src.DiffChunkBytes > 0 {
		dst.DiffChunkBytes = src.DiffChunkBytes
	}
}

// MaxRetriesVal returns the MaxRetries value, defaulting to 0 if nil.
func (c *Config) MaxRetriesVal() int {
	if c.MaxRetries == nil {
		return 0
	}
	return *c.MaxRetries
}

// DiffContextLinesVal returns the configured context lines, defaulting to 1.
func (c *Config) DiffContextLinesVal() int {
	if c.DiffContextLines == nil {
		return 1
	}
	return *c.DiffContextLines
}

// TimeoutDuration parses the AgentTimeout string as a Go duration.
// Returns 0 if the string is empty or invalid.
func (c *Config) TimeoutDuration() time.Duration {
	if c.AgentTimeout == "" {
		return 0
	}
	d, err := time.ParseDuration(c.AgentTimeout)
	if err != nil {
		return 0
	}
	return d
}
