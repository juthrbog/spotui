// Package engine runs the go-librespot player inside the spotui process. It
// plays audio on this machine and shows up as a Spotify Connect device.
//
// The go-librespot daemon normally takes commands over HTTP. Here it gets a
// channel-based server instead, so commands are plain method calls and events
// arrive on a Go channel.
package engine

import (
	"context"
	"errors"
	"fmt"
	"time"

	librespot "github.com/devgianlu/go-librespot"
	"github.com/devgianlu/go-librespot/daemon"
)

// Options configures the engine.
type Options struct {
	DeviceName    string
	AudioBackend  string
	AudioDevice   string
	Bitrate       int
	InitialVolume int
	StatePath     string
	Logger        librespot.Logger
}

// AuthCode asks the user to approve the login. The engine sends it when no
// saved credentials exist. A nil URL means the login code is no longer
// needed.
type AuthCode struct {
	URL       string
	Code      string
	ExpiresAt time.Time
}

// Stopped reports that the player stopped. The engine restarts it after a
// short wait.
type Stopped struct {
	Err   error
	Retry time.Duration
}

// Engine owns the embedded player.
type Engine struct {
	opts   Options
	reqs   chan daemon.ApiRequest
	events chan any
}

// volumeSteps is the volume scale used for every volume value of the engine.
const volumeSteps = 100

// New creates an engine. Call Run to start it.
func New(opts Options) *Engine {
	return &Engine{
		opts:   opts,
		reqs:   make(chan daemon.ApiRequest),
		events: make(chan any, 256),
	}
}

// Events returns the channel of engine events: *daemon.ApiEvent, AuthCode,
// and Stopped values.
func (e *Engine) Events() <-chan any { return e.events }

// Run starts the player and blocks until ctx is cancelled. If the player stops
// because of an error, Run starts it again after a growing delay.
func (e *Engine) Run(ctx context.Context) {
	backoff := 2 * time.Second
	for {
		started := time.Now()
		err := e.runOnce(ctx)
		if ctx.Err() != nil {
			return
		}
		if err == nil {
			err = errors.New("player stopped")
		}
		if time.Since(started) > time.Minute {
			backoff = 2 * time.Second
		}
		e.opts.Logger.WithError(err).Warnf("player stopped, restarting in %s", backoff)
		e.emit(Stopped{Err: err, Retry: backoff})

		select {
		case <-ctx.Done():
			return
		case <-time.After(backoff):
		}
		backoff = min(backoff*2, time.Minute)
	}
}

func (e *Engine) runOnce(ctx context.Context) error {
	cfg := &daemon.Config{
		DeviceName:    e.opts.DeviceName,
		DeviceType:    "computer",
		AudioBackend:  e.opts.AudioBackend,
		AudioDevice:   e.opts.AudioDevice,
		Bitrate:       e.opts.Bitrate,
		VolumeSteps:   volumeSteps,
		InitialVolume: uint32(min(max(e.opts.InitialVolume, 0), volumeSteps)), // #nosec G115 -- clamped to 0..100
		SkipDebounce:  600 * time.Millisecond,
	}
	// Device auth is used only when no saved credentials exist. It gives a URL
	// and a code through SetAuthCode, which the UI shows.
	cfg.Credentials.Type = "device_auth"
	cfg.Metadata.Enabled = true

	app, err := daemon.New(&daemon.Options{
		Logger:     e.opts.Logger,
		Config:     cfg,
		StateStore: newFileStateStore(e.opts.StatePath),
		APIServer:  &chanServer{engine: e},
	})
	if err != nil {
		return fmt.Errorf("creating player: %w", err)
	}
	return app.Run(ctx)
}

func (e *Engine) emit(ev any) {
	select {
	case e.events <- ev:
	default:
		e.opts.Logger.Warnf("event queue full, dropping %T", ev)
	}
}

// call sends a request to the player and waits for its reply.
func (e *Engine) call(ctx context.Context, t daemon.ApiRequestType, data any) (any, error) {
	req, wait := daemon.NewApiRequest(t, data)
	select {
	case e.reqs <- req:
	case <-ctx.Done():
		if errors.Is(ctx.Err(), context.DeadlineExceeded) {
			return nil, ErrNotReady
		}
		return nil, ctx.Err()
	}
	return wait(ctx)
}

// ErrNotReady means the player did not take the request in time, usually
// because it is still logging in.
var ErrNotReady = errors.New("player is not ready")

// chanServer implements daemon.ApiServer with channels instead of HTTP.
type chanServer struct {
	engine *Engine
}

// Emit is called from the player loop, so it must never block.
func (s *chanServer) Emit(ev *daemon.ApiEvent) { s.engine.emit(ev) }

func (s *chanServer) Receive() <-chan daemon.ApiRequest { return s.engine.reqs }

func (s *chanServer) SetAuthCode(auth *daemon.ApiDeviceAuth) {
	if auth == nil {
		s.engine.emit(AuthCode{})
		return
	}
	s.engine.emit(AuthCode{URL: auth.Url, Code: auth.Code, ExpiresAt: auth.ExpiresAt})
}

// Close is called when a daemon instance shuts down. The channels stay open
// because the next instance reuses them.
func (s *chanServer) Close() error { return nil }
