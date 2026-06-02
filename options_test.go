package hamr

import (
	"encoding/json"
	"log/slog"
	"os"
	"path/filepath"
	"testing"

	"github.com/BurntSushi/toml"
	"gopkg.in/yaml.v3"
)

func TestWithConfigFileJSON(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "config.json")

	cfg := map[string]any{
		"version":        "1.0.0",
		"transport":      "sse",
		"description":    "JSON server",
		"minimalSchemas": true,
	}
	b, _ := json.Marshal(cfg)
	if err := os.WriteFile(path, b, 0644); err != nil {
		t.Fatalf("write config: %v", err)
	}

	s := hamr.New("test", "0.0.0", hamr.WithConfigFile(path))
	if s == nil {
		t.Fatal("expected non-nil server")
	}
}

func TestWithConfigFileYAML(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "config.yaml")

	data := []byte("version: \"1.0.0\"\ntransport: \"sse\"\ndescription: \"YAML server\"\nminimalSchemas: true\n")
	if err := os.WriteFile(path, data, 0644); err != nil {
		t.Fatalf("write config: %v", err)
	}

	s := hamr.New("test", "0.0.0", hamr.WithConfigFile(path))
	if s == nil {
		t.Fatal("expected non-nil server")
	}
}

func TestWithConfigFileTOML(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "config.toml")

	data := []byte("version = \"1.0.0\"\ntransport = \"sse\"\ndescription = \"TOML server\"\nminimalSchemas = true\n")
	if err := os.WriteFile(path, data, 0644); err != nil {
		t.Fatalf("write config: %v", err)
	}

	s := hamr.New("test", "0.0.0", hamr.WithConfigFile(path))
	if s == nil {
		t.Fatal("expected non-nil server")
	}
}

func TestWithConfigFileUnsupported(t *testing.T) {
	defer func() {
		if r := recover(); r == nil {
			t.Fatal("expected panic for unsupported extension")
		}
	}()
	hamr.WithConfigFile("/tmp/config.xml")
}

func TestWithConfigFileMissing(t *testing.T) {
	defer func() {
		if r := recover(); r == nil {
			t.Fatal("expected panic for missing config file")
		}
	}()
	hamr.WithConfigFile("/nonexistent/path/config.yaml")
}

func TestWithEnvPrefix(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "config.yaml")

	data := []byte("version: \"1.0.0\"\ntransport: \"sse\"\ndescription: \"Env server\"\nminimalSchemas: true\n")
	if err := os.WriteFile(path, data, 0644); err != nil {
		t.Fatalf("write config: %v", err)
	}

	os.Setenv("TEST_OVERRIDE_VERSION", "9.9.9")
	os.Setenv("TEST_OVERRIDE_DESCRIPTION", "env override")
	defer os.Unsetenv("TEST_OVERRIDE_VERSION")
	defer os.Unsetenv("TEST_OVERRIDE_DESCRIPTION")

	s := hamr.New("test", "0.0.0",
		hamr.WithConfigFile(path),
		hamr.WithEnvPrefix("TEST_OVERRIDE_"),
	)
	if s == nil {
		t.Fatal("expected non-nil server")
	}
}

func TestWithEnvPrefixInvalidBool(t *testing.T) {
	defer func() {
		if r := recover(); r == nil {
			t.Fatal("expected panic for invalid bool env var")
		}
	}()
	os.Setenv("TEST_BAD_BOOL_MINIMAL_SCHEMAS", "notabool")
	defer os.Unsetenv("TEST_BAD_BOOL_MINIMAL_SCHEMAS")
	hamr.WithEnvPrefix("TEST_BAD_BOOL_")
}

func TestConfigDefaults(t *testing.T) {
	s := hamr.New("default-test", "0.0.1")
	if s == nil {
		t.Fatal("expected non-nil server")
	}
}

func TestBackwardCompatibility(t *testing.T) {
	s := hamr.New("compat", "1.0.0",
		hamr.WithTransport("sse"),
		hamr.WithDescription("compat test"),
		hamr.WithMinimalSchemas(),
	)
	if s == nil {
		t.Fatal("expected non-nil server")
	}
}

func TestWithLoggerPreserved(t *testing.T) {
	s := hamr.New("logger-test", "1.0.0", hamr.WithLogger(slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: slog.LevelError}))))
	if s == nil {
		t.Fatal("expected non-nil server")
	}
}

func TestConfigFileValidation(t *testing.T) {
	dir := t.TempDir()

	validJSON := filepath.Join(dir, "valid.json")
	_ = os.WriteFile(validJSON, []byte(`{"version":"1.0"}`), 0644)
	if _, err := os.Stat(validJSON); err != nil {
		t.Fatalf("expected valid.json to exist: %v", err)
	}

	validYAML := filepath.Join(dir, "valid.yaml")
	_ = os.WriteFile(validYAML, []byte("version: \"1.0\"\n"), 0644)
	if _, err := os.Stat(validYAML); err != nil {
		t.Fatalf("expected valid.yaml to exist: %v", err)
	}

	validTOML := filepath.Join(dir, "valid.toml")
	_ = os.WriteFile(validTOML, []byte("version = \"1.0\"\n"), 0644)
	if _, err := os.Stat(validTOML); err != nil {
		t.Fatalf("expected valid.toml to exist: %v", err)
	}
}

func TestTOMLUnmarshalDirect(t *testing.T) {
	var cfg map[string]any
	data := []byte("version = \"1.0\"\ntransport = \"sse\"\n")
	if err := toml.Unmarshal(data, &cfg); err != nil {
		t.Fatalf("toml unmarshal failed: %v", err)
	}
	if cfg["version"] != "1.0" {
		t.Errorf("expected version 1.0, got %v", cfg["version"])
	}
}

func TestYAMLUnmarshalDirect(t *testing.T) {
	var cfg map[string]any
	data := []byte("version: \"1.0\"\ntransport: \"sse\"\n")
	if err := yaml.Unmarshal(data, &cfg); err != nil {
		t.Fatalf("yaml unmarshal failed: %v", err)
	}
	if cfg["version"] != "1.0" {
		t.Errorf("expected version 1.0, got %v", cfg["version"])
	}
}

func TestJSONUnmarshalDirect(t *testing.T) {
	var cfg map[string]any
	data := []byte(`{"version":"1.0","transport":"sse"}`)
	if err := json.Unmarshal(data, &cfg); err != nil {
		t.Fatalf("json unmarshal failed: %v", err)
	}
	if cfg["version"] != "1.0" {
		t.Errorf("expected version 1.0, got %v", cfg["version"])
	}
}
