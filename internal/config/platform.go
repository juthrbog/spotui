package config

import (
	"fmt"
	"os"
	"path/filepath"
)

// audioBackends lists the go-librespot audio backends that spotui supports
// on goos. The first one is the default.
func audioBackends(goos string) []string {
	switch goos {
	case "darwin":
		return []string{"audio-toolbox", "pulseaudio"}
	case "windows":
		return []string{"wasapi"}
	default:
		return []string{"pulseaudio", "alsa"}
	}
}

// backendHelp is the comment above audio_backend in the config template.
func backendHelp(goos string) string {
	switch goos {
	case "darwin":
		return `"audio-toolbox" (the macOS sound system) or "pulseaudio".`
	case "windows":
		return `"wasapi" (the Windows sound system).`
	default:
		return `"pulseaudio" (also works with PipeWire) or "alsa".`
	}
}

// stateHome returns the directory for the files that spotui writes itself.
// XDG_STATE_HOME overrides it on every system.
//
//   - Linux: ~/.local/state
//   - macOS: ~/Library/Application Support
//   - Windows: %LocalAppData%
func stateHome(goos string) (string, error) {
	if dir := os.Getenv("XDG_STATE_HOME"); dir != "" {
		return dir, nil
	}
	if goos == "windows" {
		dir := os.Getenv("LocalAppData")
		if dir == "" {
			return "", fmt.Errorf("%%LocalAppData%% is not set")
		}
		return dir, nil
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return "", fmt.Errorf("finding home directory: %w", err)
	}
	if goos == "darwin" {
		return filepath.Join(home, "Library", "Application Support"), nil
	}
	return filepath.Join(home, ".local", "state"), nil
}
