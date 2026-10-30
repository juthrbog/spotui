package ui

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"
	"github.com/charmbracelet/x/ansi"
	"github.com/devgianlu/go-librespot/daemon"

	"github.com/juthrbog/spotui/internal/engine"
	"github.com/juthrbog/spotui/internal/webapi"
)

// newWebModel returns a model whose Web API client has a saved login.
func newWebModel(t *testing.T) *Model {
	t.Helper()
	path := filepath.Join(t.TempDir(), "token.json")
	if err := os.WriteFile(path, []byte(`{"access_token": "a", "refresh_token": "r"}`), 0o600); err != nil {
		t.Fatal(err)
	}
	web := webapi.New("client-id", 8898, path)
	if !web.LoggedIn() {
		t.Fatal("test token was not loaded")
	}
	m := New(context.Background(), engine.New(engine.Options{}), web, Info{})
	m.Update(tea.WindowSizeMsg{Width: 120, Height: 30})
	m.Update(statusMsg{st: &daemon.ApiStatus{Username: "jakob"}})
	return m
}

func albums(names ...string) []webapi.Album {
	out := make([]webapi.Album, len(names))
	for i, n := range names {
		out[i] = webapi.Album{URI: "spotify:album:" + n, Name: n, ReleaseDate: "2020-01-01"}
	}
	return out
}

func rowTexts(l *list) []string {
	var out []string
	for _, r := range l.rows {
		out = append(out, strings.Join(r.cols, " "))
	}
	return out
}

func TestOpenArtistWithWebAPIShowsDiscography(t *testing.T) {
	m := newWebModel(t)
	m.openArtist("spotify:artist:x", "Band")
	p, ok := m.current().(*artistPage)
	if !ok {
		t.Fatalf("openArtist opened %T, want *artistPage", m.current())
	}

	m.Update(artistAlbumsMsg{page: p, group: webapi.GroupSingle, albums: albums("Hit"), total: 1})
	text := strings.Join(rowTexts(&p.list), "\n")
	if !strings.Contains(text, "Singles and EPs") || !strings.Contains(text, "Hit") || !strings.Contains(text, "Loading…") {
		t.Errorf("rows after one group:\n%s", text)
	}

	m.Update(artistAlbumsMsg{page: p, group: webapi.GroupAlbum, albums: albums("First", "Second"), total: 5})
	m.Update(artistAlbumsMsg{page: p, group: webapi.GroupCompilation})
	m.Update(artistAlbumsMsg{page: p, group: webapi.GroupAppearsOn})
	text = strings.Join(rowTexts(&p.list), "\n")
	for _, want := range []string{"▶ Play Band", "All tracks", "Albums", "First", "Showing the first 2 of 5."} {
		if !strings.Contains(text, want) {
			t.Errorf("rows do not contain %q:\n%s", want, text)
		}
	}
	for _, unwanted := range []string{"Loading…", "Compilations", "Appears On"} {
		if strings.Contains(text, unwanted) {
			t.Errorf("rows contain %q:\n%s", unwanted, text)
		}
	}
	if strings.Index(text, "Albums") > strings.Index(text, "Singles and EPs") {
		t.Errorf("Albums is not above Singles and EPs:\n%s", text)
	}
	checkFrame(t, m, 120, 30, "artist page")
}

func TestArtistPageEnterOpensAlbumAndAllTracks(t *testing.T) {
	m := newWebModel(t)
	p := newArtistPage("spotify:artist:x", "Band")
	m.push(p)
	m.Update(artistAlbumsMsg{page: p, group: webapi.GroupAlbum, albums: albums("First"), total: 1})

	pressKey(m, 'j') // All tracks
	m.Update(tea.KeyPressMsg{Code: tea.KeyEnter})
	if got := currentTracks(t, m); got.uri != "spotify:artist:x" || got.hint != "" {
		t.Errorf("All tracks opened %q with hint %q", got.uri, got.hint)
	}
	m.pop()

	pressKey(m, 'G') // the album
	m.Update(tea.KeyPressMsg{Code: tea.KeyEnter})
	if got := currentTracks(t, m); got.uri != "spotify:album:First" || got.name != "First" {
		t.Errorf("album row opened %q (%q)", got.uri, got.name)
	}
}

func TestArtistPageErrorKeepsArtistRows(t *testing.T) {
	m := newWebModel(t)
	p := newArtistPage("spotify:artist:x", "Band")
	m.push(p)
	m.Update(artistAlbumsMsg{page: p, group: webapi.GroupAlbum, err: errors.New("boom")})
	m.Update(artistAlbumsMsg{page: p, group: webapi.GroupSingle, albums: albums("Hit"), total: 1})

	text := strings.Join(rowTexts(&p.list), "\n")
	for _, want := range []string{"▶ Play Band", "All tracks", "Could not load the discography: boom", "Hit"} {
		if !strings.Contains(text, want) {
			t.Errorf("rows do not contain %q:\n%s", want, text)
		}
	}
	if strings.Contains(text, "Loading…") {
		t.Errorf("rows still show Loading… after an error:\n%s", text)
	}
	if !m.flashErr || !strings.Contains(m.flash, "boom") {
		t.Errorf("flash = %q, want the error", m.flash)
	}
}

func TestWebLoginReloadsArtistPages(t *testing.T) {
	m := newWebModel(t)
	failed := newArtistPage("spotify:artist:x", "Band")
	m.push(failed)
	m.Update(artistAlbumsMsg{page: failed, group: webapi.GroupAlbum, err: errors.New("boom")})
	fallback := newTracksPage("spotify:artist:y", "Other")
	fallback.hint = "Log in to the Web API with L to see the discography."
	m.push(fallback)

	m.webLoginTried = true
	m.Update(webLoginDoneMsg{})
	if failed.err != nil || !strings.Contains(strings.Join(rowTexts(&failed.list), "\n"), "Loading…") {
		t.Errorf("failed artist page did not load again: err %v, rows %q", failed.err, rowTexts(&failed.list))
	}
	got, ok := m.current().(*artistPage)
	if !ok || got.uri != "spotify:artist:y" || got.name != "Other" {
		t.Errorf("fallback track list became %T %+v, want the artist page of spotify:artist:y", m.current(), m.current())
	}
}

func TestGoToContextOpensArtistPage(t *testing.T) {
	m := newWebModel(t)
	uri, name := "spotify:artist:x", "Band"
	m.Update(statusMsg{st: &daemon.ApiStatus{Username: "jakob", ContextUri: &uri, ContextName: &name}})

	pressKey(m, 'o')
	p, ok := m.current().(*artistPage)
	if !ok || p.uri != uri {
		t.Fatalf("o opened %T, want the artist page", m.current())
	}
	m.Update(tea.KeyPressMsg{Code: '2', Text: "2"})
	pressKey(m, 'o')
	if m.current() != p {
		t.Errorf("o opened %T, want the open artist page", m.current())
	}
}

func TestOpenArtistWithoutWebAPIShowsTracksWithHint(t *testing.T) {
	m := newGoToModel(t, nil)
	m.openArtist("spotify:artist:x", "Band")
	p := currentTracks(t, m)
	if p.uri != "spotify:artist:x" || p.hint == "" {
		t.Fatalf("opened %q with hint %q", p.uri, p.hint)
	}
	m.Update(contextTracksMsg{page: p, resp: &daemon.ApiContextTracks{
		Ready: true, Length: 1, Cached: 1, Tracks: []daemon.ApiContextTrackItem{{Uri: trackOne.Uri, Track: trackOne}},
	}})
	frame := ansi.Strip(checkFrame(t, m, 100, 30, "artist tracks with hint"))
	if !strings.Contains(frame, "discography") || !strings.Contains(frame, "First Song") {
		t.Errorf("frame does not show the hint and the track:\n%s", frame)
	}
}
