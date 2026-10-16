// Command spotui is a terminal Spotify player. It plays audio on this machine
// through an embedded go-librespot player.
package main

import (
	"context"
	"flag"
	"fmt"
	"log/slog"
	"os"
	"strconv"
	"time"

	tea "charm.land/bubbletea/v2"

	"github.com/juthrbog/spotui/internal/config"
	"github.com/juthrbog/spotui/internal/engine"
	"github.com/juthrbog/spotui/internal/ui"
	"github.com/juthrbog/spotui/internal/version"
	"github.com/juthrbog/spotui/internal/webapi"
)

// maxLogSize is the size at which the log file is cleared on start.
const maxLogSize = 10 << 20

func main() {
	if err := run(); err != nil {
		fmt.Fprintln(os.Stderr, "spotui:", err)
		os.Exit(1)
	}
}

func run() error {
	logout := flag.Bool("logout", false, "forget the saved Spotify logins and exit")
	debug := flag.Bool("debug", false, "write debug messages to the log file")
	showVersion := flag.Bool("version", false, "print the version and exit")
	flag.Parse()

	if *showVersion {
		fmt.Println(version.String())
		return nil
	}

	paths, err := config.ResolvePaths()
	if err != nil {
		return err
	}
	if err := paths.EnsureDirs(); err != nil {
		return err
	}
	cfg, err := config.Load(paths.ConfigFile)
	if err != nil {
		return err
	}
	web := webapi.New(cfg.ClientID, cfg.RedirectPort, paths.WebAPIToken)

	if *logout {
		if err := engine.Logout(paths.PlayerState); err != nil {
			return err
		}
		if err := web.Logout(); err != nil {
			return err
		}
		fmt.Println("Logged out. The next start asks you to log in again.")
		return nil
	}

	logFile, err := openLog(paths.LogFile)
	if err != nil {
		return err
	}
	defer func() { _ = logFile.Close() }()
	level := slog.LevelInfo
	if *debug {
		level = slog.LevelDebug
	}
	logger := slog.New(slog.NewTextHandler(logFile, &slog.HandlerOptions{Level: level}))
	logger.Info("starting", "version", version.String())

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	eng := engine.New(engine.Options{
		DeviceName:    cfg.DeviceName,
		AudioBackend:  cfg.AudioBackend,
		AudioDevice:   cfg.AudioDevice,
		Bitrate:       cfg.Bitrate,
		InitialVolume: cfg.InitialVolume,
		StatePath:     paths.PlayerState,
		Logger:        engine.NewLogger(logger),
	})
	engineDone := make(chan struct{})
	go func() {
		eng.Run(ctx)
		close(engineDone)
	}()

	model := ui.New(ctx, eng, web, ui.Info{
		ConfigFile:  paths.ConfigFile,
		LogFile:     paths.LogFile,
		RedirectURL: "http://127.0.0.1:" + strconv.Itoa(cfg.RedirectPort) + "/callback",
		CoverArt:    cfg.CoverArt,
	})
	_, runErr := tea.NewProgram(model).Run()

	// Stop the player so the audio device is released before exit.
	cancel()
	select {
	case <-engineDone:
	case <-time.After(3 * time.Second):
	}
	return runErr
}

func openLog(path string) (*os.File, error) {
	flags := os.O_CREATE | os.O_WRONLY | os.O_APPEND
	if info, err := os.Stat(path); err == nil && info.Size() > maxLogSize {
		flags = os.O_CREATE | os.O_WRONLY | os.O_TRUNC
	}
	f, err := os.OpenFile(path, flags, 0o600) // #nosec G304 -- path is built from the XDG state directory
	if err != nil {
		return nil, fmt.Errorf("opening log file: %w", err)
	}
	return f, nil
}
