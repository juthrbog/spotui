package engine

import (
	"context"
	"errors"
	"testing"
)

func TestCanonicalTrackURIs(t *testing.T) {
	// A 21-character id gets padded to the canonical 22 characters.
	got, err := canonicalTrackURIs([]string{"spotify:track:4uLU6hMCjMI75M1A2tKUQC", "spotify:track:1uLU6hMCjMI75M1A2tKUQ"})
	if err != nil {
		t.Fatal(err)
	}
	if got[0] != "spotify:track:4uLU6hMCjMI75M1A2tKUQC" || got[1] != "spotify:track:01uLU6hMCjMI75M1A2tKUQ" {
		t.Errorf("canonical = %q", got)
	}
}

func TestCanonicalTrackURIsRejectsBadInput(t *testing.T) {
	many := make([]string, maxLibraryURIs+1)
	for i := range many {
		many[i] = "spotify:track:4uLU6hMCjMI75M1A2tKUQC"
	}
	cases := map[string][]string{
		"empty":    nil,
		"too many": many,
		"album":    {"spotify:album:4uLU6hMCjMI75M1A2tKUQC"},
		"episode":  {"spotify:episode:4uLU6hMCjMI75M1A2tKUQC"},
		"local":    {"spotify:local:a:b:c:1"},
		"garbage":  {"not a uri"},
	}
	for name, uris := range cases {
		if _, err := canonicalTrackURIs(uris); err == nil {
			t.Errorf("%s: no error", name)
		}
	}
}

// TestLibraryWrappersCheckBeforeSending makes sure that bad input never
// reaches the player. The engine has no player here, so a request that is
// sent would block until the context ends.
func TestLibraryWrappersCheckBeforeSending(t *testing.T) {
	e := New(Options{})
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if err := e.SetLiked(ctx, nil, true); err == nil || err == context.Canceled {
		t.Errorf("SetLiked(nil) = %v, want a check error", err)
	}
	if _, err := e.LikedStates(ctx, []string{"spotify:album:4uLU6hMCjMI75M1A2tKUQC"}); err == nil || err == context.Canceled {
		t.Errorf("LikedStates(album) = %v, want a check error", err)
	}
	const (
		playlist = "spotify:playlist:37i9dQZF1DXcBWIGoYBM5M"
		track    = "spotify:track:4uLU6hMCjMI75M1A2tKUQC"
	)
	bad := map[string][2]any{
		"album as playlist":  {"spotify:album:4uLU6hMCjMI75M1A2tKUQC", []string{track}},
		"Liked Songs":        {"spotify:user:jakob:collection", []string{track}},
		"no items":           {playlist, []string(nil)},
		"album as item":      {playlist, []string{"spotify:album:4uLU6hMCjMI75M1A2tKUQC"}},
		"local file as item": {playlist, []string{"spotify:local:a:b:c:1"}},
	}
	for name, in := range bad {
		if err := e.PlaylistAddTracks(ctx, in[0].(string), in[1].([]string)); err == nil || err == context.Canceled {
			t.Errorf("PlaylistAddTracks %s = %v, want a check error", name, err)
		}
	}
	pos := func(i int) *int { return &i }
	removes := map[string]struct {
		playlist, uri string
		position      *int
	}{
		"negative position": {playlist, track, pos(-1)},
		"album as playlist": {"spotify:album:4uLU6hMCjMI75M1A2tKUQC", track, nil},
		"album as item":     {playlist, "spotify:album:4uLU6hMCjMI75M1A2tKUQC", nil},
		"local file":        {playlist, "spotify:local:a:b:c:1", nil},
	}
	for name, in := range removes {
		if err := e.PlaylistRemoveTrack(ctx, in.playlist, in.uri, in.position); err == nil || errors.Is(err, context.Canceled) {
			t.Errorf("PlaylistRemoveTrack %s = %v, want a check error", name, err)
		}
	}
	for _, p := range []*int{nil, pos(0)} {
		if err := e.PlaylistRemoveTrack(ctx, playlist, track, p); !errors.Is(err, context.Canceled) {
			t.Errorf("PlaylistRemoveTrack at position %v = %v, want the error of the canceled context", p, err)
		}
	}

	for _, uri := range []string{"spotify:album:4uLU6hMCjMI75M1A2tKUQC", "spotify:local:a:b:c:1", ""} {
		if err := e.AddToQueue(ctx, uri); err == nil || errors.Is(err, context.Canceled) {
			t.Errorf("AddToQueue(%q) = %v, want a check error", uri, err)
		}
	}
	if err := e.AddToQueue(ctx, track); !errors.Is(err, context.Canceled) {
		t.Errorf("AddToQueue(track) = %v, want the error of the canceled context", err)
	}

	// An episode is allowed, so the request gets to the (missing) player.
	if err := e.PlaylistAddTracks(ctx, playlist, []string{"spotify:episode:4uLU6hMCjMI75M1A2tKUQC"}); !errors.Is(err, context.Canceled) {
		t.Errorf("PlaylistAddTracks with an episode = %v, want the error of the canceled context", err)
	}
}
