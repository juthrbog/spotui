package ui

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"
	"github.com/devgianlu/go-librespot/daemon"

	"github.com/juthrbog/spotui/internal/engine"
	"github.com/juthrbog/spotui/internal/webapi"
)

// newScopedWebModel returns a model whose Web API login allows changes to
// the library.
func newScopedWebModel(t *testing.T) *Model {
	t.Helper()
	path := filepath.Join(t.TempDir(), "token.json")
	tok := `{"access_token": "a", "refresh_token": "r", "scope": "user-library-read user-library-modify"}`
	if err := os.WriteFile(path, []byte(tok), 0o600); err != nil {
		t.Fatal(err)
	}
	web := webapi.New("client-id", 8898, path)
	m := New(context.Background(), engine.New(engine.Options{}), web, Info{})
	m.Update(tea.WindowSizeMsg{Width: 100, Height: 30})
	m.Update(statusMsg{st: &daemon.ApiStatus{Username: "jakob"}})
	return m
}

func TestAlbumOf(t *testing.T) {
	cases := map[string]struct {
		item row
		want string
	}{
		"album row":        {row{uri: "spotify:album:a", label: "Rec"}, "spotify:album:a"},
		"track":            {trackRow(trackOne), "spotify:album:a1"},
		"episode":          {row{uri: "spotify:episode:e", album: ref{uri: "spotify:show:s"}}, ""},
		"track still load": {row{uri: "spotify:track:9"}, ""},
	}
	for name, c := range cases {
		if got := albumOf(&c.item).uri; got != c.want {
			t.Errorf("%s: %q, want %q", name, got, c.want)
		}
	}
}

func TestToggleAlbumChecks(t *testing.T) {
	album := &row{uri: "spotify:album:a", label: "Rec"}

	m := newGoToModel(t, nil)
	if m.toggleAlbumOf(&row{uri: "spotify:track:9"}); !strings.Contains(m.flash, "No album") {
		t.Errorf("no album: flash = %q", m.flash)
	}
	if m.toggleAlbumOf(album); !strings.Contains(m.flash, "Push L to log in.") {
		t.Errorf("no Web API: flash = %q", m.flash)
	}

	m = newWebModel(t) // a login from before the new scope
	if m.toggleAlbumOf(album); !strings.Contains(m.flash, "Push L to log in again") {
		t.Errorf("old login: flash = %q", m.flash)
	}

	m = newScopedWebModel(t)
	if cmd := m.toggleAlbumOf(album); cmd == nil || m.flash != "" || !m.albumPending[album.uri] {
		t.Errorf("ready: cmd = %v, flash = %q, pending = %v", cmd, m.flash, m.albumPending[album.uri])
	}
	if m.toggleAlbumOf(album); !strings.Contains(m.flash, "Still changing") {
		t.Errorf("second press: flash = %q", m.flash)
	}
}

func TestAlbumSavedDone(t *testing.T) {
	m := newScopedWebModel(t)
	m.albumPending["spotify:album:a"] = true

	if cmd := m.update(albumSavedMsg{uri: "spotify:album:a", name: "Rec", saved: true}); cmd == nil {
		t.Error("the saved albums did not load again")
	}
	if !m.albums.saved["spotify:album:a"] || m.albumPending["spotify:album:a"] || m.flash != "Saved Rec to your albums." {
		t.Errorf("saved = %v, pending = %v, flash = %q", m.albums.saved["spotify:album:a"], m.albumPending["spotify:album:a"], m.flash)
	}

	m.update(albumSavedMsg{uri: "spotify:album:a", name: "Rec", saved: false})
	if m.albums.saved["spotify:album:a"] || m.flash != "Removed Rec from your albums." {
		t.Errorf("after remove: saved = %v, flash = %q", m.albums.saved["spotify:album:a"], m.flash)
	}

	cases := map[error]string{
		fmt.Errorf("x: %w", webapi.ErrMissingScope): "Push L to log in again",
		webapi.ErrNotLoggedIn:                       "Push L to log in.",
		errors.New("boom"):                          "Could not change your albums: boom",
	}
	for err, want := range cases {
		m.update(albumSavedMsg{uri: "spotify:album:a", name: "Rec", saved: true, err: err})
		if !strings.Contains(m.flash, want) || !m.flashErr {
			t.Errorf("%v: flash = %q, want %q", err, m.flash, want)
		}
		if m.albums.saved["spotify:album:a"] {
			t.Errorf("%v: a failed save changed the state", err)
		}
	}
}

func TestSavedAlbumsAreKnown(t *testing.T) {
	m := newScopedWebModel(t)
	if got := albumLabel(m.albums, "spotify:album:a"); got != "Save or remove album" {
		t.Errorf("label before the load = %q", got)
	}

	m.Update(albumsMsg{items: []webapi.Album{{URI: "spotify:album:a", Name: "Rec"}}})
	if got := albumLabel(m.albums, "spotify:album:a"); got != "Remove album" {
		t.Errorf("saved album: label = %q", got)
	}
	// The load had every saved album, so other albums are not saved.
	if got := albumLabel(m.albums, "spotify:album:b"); got != "Save album" {
		t.Errorf("other album: label = %q", got)
	}

	// The album was removed in another app: the next load drops it.
	m.Update(albumsMsg{items: nil})
	if got := albumLabel(m.albums, "spotify:album:a"); got != "Save album" {
		t.Errorf("after a reload without it: label = %q", got)
	}

	// A failed load keeps what is known.
	m.update(albumSavedMsg{uri: "spotify:album:a", name: "Rec", saved: true})
	m.Update(albumsMsg{err: errors.New("boom")})
	if got := albumLabel(m.albums, "spotify:album:a"); got != "Remove album" {
		t.Errorf("after a failed load: label = %q", got)
	}
}

func TestAlbumStatesAtCap(t *testing.T) {
	var s albumStates
	s.loaded(make([]webapi.Album, webapi.MaxSavedAlbums))
	if _, known := s.state("spotify:album:z"); known {
		t.Error("an album outside a capped load counts as known")
	}
}

func TestSaveAlbumNeedsLoginInMenu(t *testing.T) {
	m := newGoToModel(t, nil)
	openTracks(m, trackOne)
	pressKey(m, 'a')
	for i, it := range m.menu.items {
		if it.label == "Save or remove album" {
			if !it.needsWeb {
				t.Error("the album action does not need the Web API")
			}
			m.menu.cursor = i
		}
	}
	m.Update(tea.KeyPressMsg{Code: tea.KeyEnter})
	if !strings.Contains(m.flash, "needs the Web API") {
		t.Errorf("flash = %q", m.flash)
	}
}
