// Package config loads the spotui settings file and resolves the paths where
// spotui keeps its state.
package config

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"slices"
	"strings"

	"github.com/BurntSushi/toml"
)

const appName = "spotui"

// Config holds the user settings from config.toml.
type Config struct {
	// ClientID is the client ID of the user's own Spotify developer app. It is
	// used only for Web API calls (search and saved albums).
	ClientID string `toml:"client_id"`
	// RedirectPort is the loopback port of the Web API login callback. The
	// developer app must list http://127.0.0.1:<port>/callback.
	RedirectPort int `toml:"redirect_port"`

	DeviceName    string `toml:"device_name"`
	AudioBackend  string `toml:"audio_backend"`
	AudioDevice   string `toml:"audio_device"`
	Bitrate       int    `toml:"bitrate"`
	InitialVolume int    `toml:"initial_volume"`

	// CoverArt selects how cover images are drawn: "auto", "kitty",
	// "blocks", or "off".
	CoverArt string `toml:"cover_art"`
}

// Paths are the files spotui reads and writes.
type Paths struct {
	ConfigDir   string
	StateDir    string
	ConfigFile  string
	PlayerState string
	WebAPIToken string
	LogFile     string
}

func defaults() Config {
	return Config{
		RedirectPort:  8898,
		DeviceName:    appName,
		AudioBackend:  audioBackends(runtime.GOOS)[0],
		Bitrate:       320,
		InitialVolume: 50,
		CoverArt:      "auto",
	}
}

// ResolvePaths returns the spotui paths. The config file is in the user
// config directory. The login, the token, and the log are in the state
// directory of the operating system.
func ResolvePaths() (Paths, error) {
	configHome, err := os.UserConfigDir()
	if err != nil {
		return Paths{}, fmt.Errorf("finding config directory: %w", err)
	}
	stateHome, err := stateHome(runtime.GOOS)
	if err != nil {
		return Paths{}, err
	}

	configDir := filepath.Join(configHome, appName)
	stateDir := filepath.Join(stateHome, appName)
	return Paths{
		ConfigDir:   configDir,
		StateDir:    stateDir,
		ConfigFile:  filepath.Join(configDir, "config.toml"),
		PlayerState: filepath.Join(stateDir, "player-state.json"),
		WebAPIToken: filepath.Join(stateDir, "webapi-token.json"),
		LogFile:     filepath.Join(stateDir, "spotui.log"),
	}, nil
}

// EnsureDirs creates the config and state directories.
func (p Paths) EnsureDirs() error {
	for _, dir := range []string{p.ConfigDir, p.StateDir} {
		if err := os.MkdirAll(dir, 0o700); err != nil {
			return fmt.Errorf("creating %s: %w", dir, err)
		}
	}
	return nil
}

// Load reads the config file. A missing file gives the defaults, and a
// commented template is written so the user can find and edit it.
// SPOTUI_CLIENT_ID overrides client_id.
func Load(path string) (Config, error) {
	cfg := defaults()

	_, err := toml.DecodeFile(path, &cfg)
	switch {
	case errors.Is(err, os.ErrNotExist):
		if err := os.WriteFile(path, []byte(template(runtime.GOOS)), 0o600); err != nil {
			return cfg, fmt.Errorf("writing config template: %w", err)
		}
	case err != nil:
		return cfg, fmt.Errorf("reading %s: %w", path, err)
	}

	if id := os.Getenv("SPOTUI_CLIENT_ID"); id != "" {
		cfg.ClientID = id
	}

	switch cfg.Bitrate {
	case 96, 160, 320:
	default:
		return cfg, fmt.Errorf("bitrate must be 96, 160, or 320, not %d", cfg.Bitrate)
	}
	if cfg.InitialVolume < 0 || cfg.InitialVolume > 100 {
		return cfg, fmt.Errorf("initial_volume must be between 0 and 100, not %d", cfg.InitialVolume)
	}
	if backends := audioBackends(runtime.GOOS); !slices.Contains(backends, cfg.AudioBackend) {
		return cfg, fmt.Errorf("audio_backend must be one of %q on this system, not %q", backends, cfg.AudioBackend)
	}
	switch cfg.CoverArt {
	case "auto", "kitty", "blocks", "off":
	default:
		return cfg, fmt.Errorf(`cover_art must be "auto", "kitty", "blocks", or "off", not %q`, cfg.CoverArt)
	}
	if cfg.RedirectPort <= 0 || cfg.RedirectPort > 65535 {
		return cfg, fmt.Errorf("redirect_port must be between 1 and 65535, not %d", cfg.RedirectPort)
	}
	return cfg, nil
}

// template returns the commented config file that is written on the first
// start.
func template(goos string) string {
	return strings.NewReplacer(
		"{{backend}}", audioBackends(goos)[0],
		"{{backend_help}}", backendHelp(goos),
	).Replace(templateText)
}

const templateText = `# spotui settings

# Client ID of your own Spotify developer app. spotui uses it only for search
# and saved albums. Create the app at https://developer.spotify.com/dashboard,
# select "Web API", and add this redirect URI:
#   http://127.0.0.1:8898/callback
# The SPOTUI_CLIENT_ID environment variable overrides this value.
client_id = ""

# Port of the login callback. If you change it, change the redirect URI too.
redirect_port = 8898

# Name shown in the Spotify Connect device list.
device_name = "spotui"

# {{backend_help}}
audio_backend = "{{backend}}"

# Output device. Empty means the system default.
audio_device = ""

# Stream quality in kbit/s: 96, 160, or 320.
bitrate = 320

# Volume on first start, from 0 to 100.
initial_volume = 50

# How cover art is drawn:
#   "auto"   - sharp images if the terminal supports kitty graphics, else blocks
#   "kitty"  - sharp images (kitty, Ghostty)
#   "blocks" - colored half-block characters, works in any true-color terminal
#   "off"    - no cover art
cover_art = "auto"
`
