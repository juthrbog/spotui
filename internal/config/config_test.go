package config

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/BurntSushi/toml"
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
	for _, body := range []string{`bitrate = 128`, `initial_volume = 101`, `redirect_port = 0`, `cover_art = "ascii"`, `audio_backend = "jack"`} {
		path := filepath.Join(t.TempDir(), "config.toml")
		if err := os.WriteFile(path, []byte(body), 0o600); err != nil {
			t.Fatal(err)
		}
		if _, err := Load(path); err == nil {
			t.Errorf("%s: no error", body)
		}
	}
}

func TestPlatformDefaults(t *testing.T) {
	cases := []struct {
		goos, backend string
	}{
		{"linux", "pulseaudio"},
		{"darwin", "audio-toolbox"},
		{"windows", "wasapi"},
	}
	for _, c := range cases {
		if got := audioBackends(c.goos)[0]; got != c.backend {
			t.Errorf("%s: default backend = %q, want %q", c.goos, got, c.backend)
		}
		tmpl := template(c.goos)
		var cfg Config
		if _, err := toml.Decode(tmpl, &cfg); err != nil {
			t.Fatalf("%s: template does not parse: %v", c.goos, err)
		}
		if cfg.AudioBackend != c.backend {
			t.Errorf("%s: template audio_backend = %q, want %q", c.goos, cfg.AudioBackend, c.backend)
		}
		if strings.Contains(tmpl, "{{") {
			t.Errorf("%s: template has an unreplaced placeholder", c.goos)
		}
	}
}

func TestStateHome(t *testing.T) {
	t.Setenv("XDG_STATE_HOME", "")
	t.Setenv("HOME", "/home/u")
	t.Setenv("LocalAppData", `C:\Users\u\AppData\Local`)
	cases := map[string]string{
		"linux":   filepath.Join("/home/u", ".local", "state"),
		"darwin":  filepath.Join("/home/u", "Library", "Application Support"),
		"windows": `C:\Users\u\AppData\Local`,
	}
	for goos, want := range cases {
		got, err := stateHome(goos)
		if err != nil || got != want {
			t.Errorf("%s: stateHome = %q, %v, want %q", goos, got, err, want)
		}
	}

	t.Setenv("XDG_STATE_HOME", "/custom")
	if got, _ := stateHome("darwin"); got != "/custom" {
		t.Errorf("XDG_STATE_HOME is not used: %q", got)
	}
}
