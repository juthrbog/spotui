package engine

import (
	"context"
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
}
