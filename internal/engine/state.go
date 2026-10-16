package engine

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"

	librespot "github.com/devgianlu/go-librespot"
)

// fileStateStore keeps the player state, including the saved login, in one
// JSON file. It is based on the store in go-librespot's cmd/daemon, which
// cannot be imported.
type fileStateStore struct {
	path string
}

func newFileStateStore(path string) *fileStateStore {
	return &fileStateStore{path: path}
}

func (s *fileStateStore) Load() (*librespot.AppState, error) {
	state := &librespot.AppState{}
	content, err := os.ReadFile(s.path)
	switch {
	case errors.Is(err, os.ErrNotExist):
		return state, nil
	case err != nil:
		return nil, fmt.Errorf("reading player state: %w", err)
	}
	if err := json.Unmarshal(content, state); err != nil {
		return nil, fmt.Errorf("parsing player state: %w", err)
	}
	return state, nil
}

// Save writes the state to a temporary file and renames it, so a crash never
// leaves a half-written file.
func (s *fileStateStore) Save(state *librespot.AppState) error {
	state.Lock()
	defer state.Unlock()

	tmp, err := os.CreateTemp(filepath.Dir(s.path), filepath.Base(s.path)+".*.tmp")
	if err != nil {
		return fmt.Errorf("creating temporary state file: %w", err)
	}
	if err := json.NewEncoder(tmp).Encode(state); err != nil {
		_ = tmp.Close()
		_ = os.Remove(tmp.Name())
		return fmt.Errorf("writing player state: %w", err)
	}
	if err := tmp.Close(); err != nil {
		_ = os.Remove(tmp.Name())
		return fmt.Errorf("closing temporary state file: %w", err)
	}
	if err := os.Rename(tmp.Name(), s.path); err != nil {
		_ = os.Remove(tmp.Name())
		return fmt.Errorf("replacing player state: %w", err)
	}
	return nil
}

// Logout removes the saved Spotify login from the state file. The next start
// asks the user to log in again.
func Logout(statePath string) error {
	store := newFileStateStore(statePath)
	state, err := store.Load()
	if err != nil {
		return err
	}
	if len(state.Credentials.Data) == 0 {
		return nil
	}
	state.Credentials.Username = ""
	state.Credentials.Data = nil
	return store.Save(state)
}

// slogLogger adapts slog to the go-librespot Logger interface.
type slogLogger struct {
	l *slog.Logger
}

// LevelTrace is below slog's debug level. go-librespot logs every packet at
// this level.
const LevelTrace = slog.Level(-8)

// NewLogger returns a go-librespot logger that writes to l.
func NewLogger(l *slog.Logger) librespot.Logger { return slogLogger{l: l} }

func (s slogLogger) logf(level slog.Level, format string, args ...any) {
	s.l.Log(context.Background(), level, fmt.Sprintf(format, args...))
}

func (s slogLogger) Tracef(format string, args ...any) { s.logf(LevelTrace, format, args...) }
func (s slogLogger) Debugf(format string, args ...any) { s.logf(slog.LevelDebug, format, args...) }
func (s slogLogger) Infof(format string, args ...any)  { s.logf(slog.LevelInfo, format, args...) }
func (s slogLogger) Warnf(format string, args ...any)  { s.logf(slog.LevelWarn, format, args...) }
func (s slogLogger) Errorf(format string, args ...any) { s.logf(slog.LevelError, format, args...) }

func (s slogLogger) Trace(args ...any) { s.logf(LevelTrace, "%s", fmt.Sprint(args...)) }
func (s slogLogger) Debug(args ...any) { s.logf(slog.LevelDebug, "%s", fmt.Sprint(args...)) }
func (s slogLogger) Info(args ...any)  { s.logf(slog.LevelInfo, "%s", fmt.Sprint(args...)) }
func (s slogLogger) Warn(args ...any)  { s.logf(slog.LevelWarn, "%s", fmt.Sprint(args...)) }
func (s slogLogger) Error(args ...any) { s.logf(slog.LevelError, "%s", fmt.Sprint(args...)) }

func (s slogLogger) WithField(key string, value any) librespot.Logger {
	return slogLogger{l: s.l.With(key, value)}
}

func (s slogLogger) WithError(err error) librespot.Logger {
	return slogLogger{l: s.l.With("err", err)}
}
