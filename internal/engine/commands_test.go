package engine

import (
	"context"
	"fmt"
	"testing"
	"time"

	"github.com/devgianlu/go-librespot/daemon"
)

// The player reads the data of most requests with a type assertion and no
// check, so data of the wrong type panics it and ends spotui. Each command
// must send the type that go-librespot reads.
func TestCommandsSendTheDataTypeThePlayerReads(t *testing.T) {
	cases := []struct {
		name string
		run  func(*Engine, context.Context) error
		typ  daemon.ApiRequestType
		want string // the type of the data, as %T prints it
	}{
		{"Prev", (*Engine).Prev, daemon.ApiRequestTypePrev, "daemon.ApiPrev"},
		{"Next", (*Engine).Next, daemon.ApiRequestTypeNext, "daemon.ApiNext"},
		{"Seek", func(e *Engine, ctx context.Context) error { return e.Seek(ctx, 5000) }, daemon.ApiRequestTypeSeek, "daemon.ApiSeek"},
		{"Play", func(e *Engine, ctx context.Context) error { return e.Play(ctx, "spotify:album:1", "") }, daemon.ApiRequestTypePlay, "daemon.ApiPlay"},
		{"AddToQueue", func(e *Engine, ctx context.Context) error {
			return e.AddToQueue(ctx, "spotify:track:4uLU6hMCjMI75M1A2tKUQC")
		}, daemon.ApiRequestTypeAddToQueue, "string"},
		{"AdjustVolume", func(e *Engine, ctx context.Context) error { return e.AdjustVolume(ctx, 5) }, daemon.ApiRequestTypeSetVolume, "daemon.ApiSetVolume"},
		{"SetShuffle", func(e *Engine, ctx context.Context) error { return e.SetShuffle(ctx, true) }, daemon.ApiRequestTypeSetShufflingContext, "bool"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			e := New(Options{})
			ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
			defer cancel()

			got := make(chan daemon.ApiRequest, 1)
			go func() {
				req := <-e.reqs
				got <- req
				req.Reply(nil, nil)
			}()
			if err := c.run(e, ctx); err != nil {
				t.Fatalf("%s: %v", c.name, err)
			}
			req := <-got
			if req.Type != c.typ {
				t.Errorf("type = %s, want %s", req.Type, c.typ)
			}
			if typ := fmt.Sprintf("%T", req.Data); typ != c.want {
				t.Errorf("data is %s, want %s", typ, c.want)
			}
		})
	}
}
