package config

import (
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestDefault(t *testing.T) {
	d := Default()
	if d.AgentTimeout != "5m" {
		t.Errorf("expected timeout '5m', got %q", d.AgentTimeout)
	}
	if d.MaxRetries != 1 {
		t.Errorf("expected retries 1, got %d", d.MaxRetries)
	}
	if d.DiffWarnBytes != 153600 {
		t.Errorf("expected warn 153600, got %d", d.DiffWarnBytes)
	}
	if d.DiffChunkBytes != 307200 {
		t.Errorf("expected chunk 307200, got %d", d.DiffChunkBytes)
	}
}

func TestLoad_ValidYAML(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, ".prism.yml")
	content := `
roles:
  - sentinel
  - architect
model: sonnet
format: md
agent_timeout: "2m"
max_retries: 3
diff_warn_bytes: 100000
diff_chunk_bytes: 200000
`
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}

	cfg, err := Load(path)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(cfg.Roles) != 2 || cfg.Roles[0] != "sentinel" {
		t.Errorf("unexpected roles: %v", cfg.Roles)
	}
	if cfg.Model != "sonnet" {
		t.Errorf("expected model 'sonnet', got %q", cfg.Model)
	}
	if cfg.Format != "md" {
		t.Errorf("expected format 'md', got %q", cfg.Format)
	}
	if cfg.AgentTimeout != "2m" {
		t.Errorf("expected timeout '2m', got %q", cfg.AgentTimeout)
	}
	if cfg.MaxRetries != 3 {
		t.Errorf("expected retries 3, got %d", cfg.MaxRetries)
	}
	if cfg.DiffWarnBytes != 100000 {
		t.Errorf("expected warn 100000, got %d", cfg.DiffWarnBytes)
	}
	if cfg.DiffChunkBytes != 200000 {
		t.Errorf("expected chunk 200000, got %d", cfg.DiffChunkBytes)
	}
}

func TestLoad_PartialYAML(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, ".prism.yml")
	if err := os.WriteFile(path, []byte("model: opus\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	cfg, err := Load(path)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if cfg.Model != "opus" {
		t.Errorf("expected model 'opus', got %q", cfg.Model)
	}
	// Unset fields should be zero values.
	if len(cfg.Roles) != 0 {
		t.Errorf("expected empty roles, got %v", cfg.Roles)
	}
	if cfg.MaxRetries != 0 {
		t.Errorf("expected retries 0, got %d", cfg.MaxRetries)
	}
}

func TestLoad_FileNotFound(t *testing.T) {
	cfg, err := Load("/nonexistent/.prism.yml")
	if err != nil {
		t.Fatalf("expected no error for missing file, got: %v", err)
	}
	// Should return zero-value config.
	if cfg.Model != "" || len(cfg.Roles) != 0 {
		t.Errorf("expected zero-value config, got: %+v", cfg)
	}
}

func TestLoad_MalformedYAML(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, ".prism.yml")
	if err := os.WriteFile(path, []byte("roles: [not closed"), 0o644); err != nil {
		t.Fatal(err)
	}

	_, err := Load(path)
	if err == nil {
		t.Fatal("expected error for malformed YAML")
	}
}

func TestMerge_CLIOverridesFileOverridesDefaults(t *testing.T) {
	def := Config{
		AgentTimeout:  "5m",
		MaxRetries:    1,
		DiffWarnBytes: 150000,
	}
	file := Config{
		Model:        "sonnet",
		AgentTimeout: "3m",
	}
	cli := Config{
		AgentTimeout: "1m",
	}

	merged := Merge(&def, &file, &cli)

	if merged.Model != "sonnet" {
		t.Errorf("expected model from file 'sonnet', got %q", merged.Model)
	}
	if merged.AgentTimeout != "1m" {
		t.Errorf("expected timeout from CLI '1m', got %q", merged.AgentTimeout)
	}
	if merged.MaxRetries != 1 {
		t.Errorf("expected retries from default 1, got %d", merged.MaxRetries)
	}
	if merged.DiffWarnBytes != 150000 {
		t.Errorf("expected warn from default 150000, got %d", merged.DiffWarnBytes)
	}
}

func TestMerge_ZeroValuesDoNotOverride(t *testing.T) {
	def := Config{MaxRetries: 2, Model: "opus"}
	file := Config{} // all zero
	cli := Config{}  // all zero

	merged := Merge(&def, &file, &cli)
	if merged.MaxRetries != 2 {
		t.Errorf("zero file/cli should not override default, got retries %d", merged.MaxRetries)
	}
	if merged.Model != "opus" {
		t.Errorf("zero file/cli should not override default, got model %q", merged.Model)
	}
}

func TestMerge_RolesOverride(t *testing.T) {
	def := Config{}
	file := Config{Roles: []string{"sentinel"}}
	cli := Config{Roles: []string{"architect", "editor"}}

	merged := Merge(&def, &file, &cli)
	if len(merged.Roles) != 2 || merged.Roles[0] != "architect" {
		t.Errorf("expected CLI roles, got %v", merged.Roles)
	}
}

func TestMerge_AllFieldsFromFile(t *testing.T) {
	def := Config{}
	file := Config{
		Roles:          []string{"sentinel"},
		Model:          "opus",
		Format:         "html",
		AgentTimeout:   "3m",
		MaxRetries:     2,
		DiffWarnBytes:  100000,
		DiffChunkBytes: 200000,
	}
	cli := Config{}

	merged := Merge(&def, &file, &cli)
	if merged.Model != "opus" {
		t.Errorf("expected model 'opus', got %q", merged.Model)
	}
	if merged.Format != "html" {
		t.Errorf("expected format 'html', got %q", merged.Format)
	}
	if merged.AgentTimeout != "3m" {
		t.Errorf("expected timeout '3m', got %q", merged.AgentTimeout)
	}
	if merged.MaxRetries != 2 {
		t.Errorf("expected retries 2, got %d", merged.MaxRetries)
	}
	if merged.DiffWarnBytes != 100000 {
		t.Errorf("expected warn 100000, got %d", merged.DiffWarnBytes)
	}
	if merged.DiffChunkBytes != 200000 {
		t.Errorf("expected chunk 200000, got %d", merged.DiffChunkBytes)
	}
	if len(merged.Roles) != 1 || merged.Roles[0] != "sentinel" {
		t.Errorf("expected roles [sentinel], got %v", merged.Roles)
	}
}

func TestTimeoutDuration_Valid(t *testing.T) {
	tests := []struct {
		input string
		want  time.Duration
	}{
		{"5m", 5 * time.Minute},
		{"30s", 30 * time.Second},
		{"2m30s", 2*time.Minute + 30*time.Second},
		{"1h", time.Hour},
	}
	for _, tt := range tests {
		cfg := Config{AgentTimeout: tt.input}
		got := cfg.TimeoutDuration()
		if got != tt.want {
			t.Errorf("TimeoutDuration(%q) = %v, want %v", tt.input, got, tt.want)
		}
	}
}

func TestTimeoutDuration_Empty(t *testing.T) {
	cfg := Config{}
	if d := cfg.TimeoutDuration(); d != 0 {
		t.Errorf("expected 0 for empty timeout, got %v", d)
	}
}

func TestTimeoutDuration_Invalid(t *testing.T) {
	cfg := Config{AgentTimeout: "not-a-duration"}
	if d := cfg.TimeoutDuration(); d != 0 {
		t.Errorf("expected 0 for invalid timeout, got %v", d)
	}
}
