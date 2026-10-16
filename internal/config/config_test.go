package config

import (
	"os"
	"path/filepath"
	"testing"
)

func TestMissingFileWritesTemplateThatMatchesDefaults(t *testing.T) {
	t.Setenv("SPOTUI_CLIENT_ID", "")
	path := filepath.Join(t.TempDir(), "config.toml")

	cfg, err := Load(path)
	if err != nil {
		t.Fatal(err)
	}
	if cfg != defaults() {
		t.Errorf("first load = %+v, want defaults", cfg)
	}
	if _, err := os.Stat(path); err != nil {
		t.Fatalf("template not written: %v", err)
	}

	// The template must parse and give the same values as the defaults.
	again, err := Load(path)
	if err != nil {
		t.Fatalf("template does not parse: %v", err)
	}
	if again != defaults() {
		t.Errorf("template = %+v, want defaults %+v", again, defaults())
	}
}

func TestEnvOverridesClientID(t *testing.T) {
	t.Setenv("SPOTUI_CLIENT_ID", "from-env")
	path := filepath.Join(t.TempDir(), "config.toml")
	if err := os.WriteFile(path, []byte(`client_id = "from-file"`), 0o600); err != nil {
		t.Fatal(err)
	}
	cfg, err := Load(path)
	if err != nil {
		t.Fatal(err)
	}
	if cfg.ClientID != "from-env" {
		t.Errorf("client_id = %q, want from-env", cfg.ClientID)
	}
}

func TestInvalidValues(t *testing.T) {
	for _, body := range []string{`bitrate = 128`, `initial_volume = 101`, `redirect_port = 0`, `cover_art = "sixel"`} {
		path := filepath.Join(t.TempDir(), "config.toml")
		if err := os.WriteFile(path, []byte(body), 0o600); err != nil {
			t.Fatal(err)
		}
		if _, err := Load(path); err == nil {
			t.Errorf("%s: no error", body)
		}
	}
}
