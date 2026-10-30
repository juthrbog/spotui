package ui

import (
	"context"
	"path/filepath"
	"testing"

	tea "charm.land/bubbletea/v2"
	"github.com/devgianlu/go-librespot/daemon"

	"github.com/juthrbog/spotui/internal/engine"
	"github.com/juthrbog/spotui/internal/webapi"
)

func newGoToModel(t *testing.T, playing *daemon.ApiTrack) *Model {
	t.Helper()
	web := webapi.New("", 8898, filepath.Join(t.TempDir(), "token.json"))
	m := New(context.Background(), engine.New(engine.Options{}), web, Info{})
	m.Update(tea.WindowSizeMsg{Width: 100, Height: 30})
	m.Update(statusMsg{st: &daemon.ApiStatus{Username: "jakob", Track: playing}})
	return m
}

func pressKey(m *Model, r rune) {
	m.Update(tea.KeyPressMsg{Code: r, Text: string(r)})
}

func openTracks(m *Model, tracks ...*daemon.ApiTrack) *tracksPage {
	p := newTracksPage("spotify:playlist:1", "Mix")
	m.push(p)
	items := make([]daemon.ApiContextTrackItem, len(tracks))
	for i, t := range tracks {
		items[i] = daemon.ApiContextTrackItem{Uri: t.Uri, Track: t}
	}
	m.Update(contextTracksMsg{page: p, resp: &daemon.ApiContextTracks{
		Ready: true, Length: len(items), Cached: len(items), Tracks: items,
	}})
	return p
}

func currentTracks(t *testing.T, m *Model) *tracksPage {
	t.Helper()
	p, ok := m.current().(*tracksPage)
	if !ok {
		t.Fatalf("current page is %T, want *tracksPage", m.current())
	}
	return p
}

var (
	trackOne = &daemon.ApiTrack{
		Uri: "spotify:track:1", Name: "First Song",
		AlbumUri: "spotify:album:a1", AlbumName: "Album One",
		ArtistUris: []string{"spotify:artist:x"}, ArtistNames: []string{"Solo Artist"},
	}
	trackTwo = &daemon.ApiTrack{
		Uri: "spotify:track:2", Name: "Second Song",
		AlbumUri: "spotify:album:a2", AlbumName: "Album Two",
		ArtistUris:  []string{"spotify:artist:y", "spotify:artist:z"},
		ArtistNames: []string{"Duo One", "Duo Two"},
	}
)

func TestGoToAlbumUsesSelectedTrackWithFilter(t *testing.T) {
	m := newGoToModel(t, nil)
	p := openTracks(m, trackOne, trackTwo)
	p.list.setFilter("second")

	pressKey(m, 'A')
	got := currentTracks(t, m)
	if got.uri != "spotify:album:a2" || got.name != "Album Two" {
		t.Errorf("A opened %q (%q), want spotify:album:a2 (Album Two)", got.uri, got.name)
	}
}

func TestGoToUsesPlayingTrackWithoutTrackSelection(t *testing.T) {
	m := newGoToModel(t, trackOne)
	m.Update(playlistsMsg{items: []daemon.ApiLibraryPlaylist{{Name: "Mix", Uri: "spotify:playlist:1"}}})
	if sel := m.current().selection(); sel == nil || isTrackURI(sel.uri) {
		t.Fatalf("library selection = %+v, want a row that is not a track", sel)
	}

	pressKey(m, 'A')
	if got := currentTracks(t, m); got.uri != "spotify:album:a1" {
		t.Errorf("A opened %q, want the album of the playing track", got.uri)
	}
	m.pop()

	pressKey(m, 'I')
	if got := currentTracks(t, m); got.uri != "spotify:artist:x" || got.name != "Solo Artist" {
		t.Errorf("I opened %q (%q), want spotify:artist:x (Solo Artist)", got.uri, got.name)
	}
}

func TestGoToArtistAsksWhenTrackHasSeveralArtists(t *testing.T) {
	m := newGoToModel(t, nil)
	openTracks(m, trackTwo)

	pressKey(m, 'I')
	chooser, ok := m.current().(*artistsPage)
	if !ok {
		t.Fatalf("I opened %T, want *artistsPage", m.current())
	}
	if chooser.title() != "Artists of Second Song" {
		t.Errorf("title = %q", chooser.title())
	}

	pressKey(m, 'j')
	m.Update(tea.KeyPressMsg{Code: tea.KeyEnter})
	if got := currentTracks(t, m); got.uri != "spotify:artist:z" || got.name != "Duo Two" {
		t.Errorf("enter opened %q (%q), want spotify:artist:z (Duo Two)", got.uri, got.name)
	}
}

func TestGoToWithoutTrackShowsMessage(t *testing.T) {
	m := newGoToModel(t, nil)
	pressKey(m, 'A')
	if _, ok := m.current().(*libraryPage); !ok {
		t.Errorf("A without a track opened %T", m.current())
	}
	if m.flash == "" || !m.flashErr {
		t.Errorf("flash = %q (error %v), want an error message", m.flash, m.flashErr)
	}
}

func TestGoToTrackStillLoading(t *testing.T) {
	m := newGoToModel(t, nil)
	p := newTracksPage("spotify:playlist:1", "Mix")
	m.push(p)
	m.Update(contextTracksMsg{page: p, resp: &daemon.ApiContextTracks{
		Ready: true, Length: 1, Tracks: []daemon.ApiContextTrackItem{{Uri: "spotify:track:9"}},
	}})

	pressKey(m, 'I')
	if m.current() != p {
		t.Errorf("I on a loading track opened %T", m.current())
	}
	if m.flash == "" {
		t.Error("no message for a track that is still loading")
	}
}

// TestGoToInTrackListIgnoresPlayingTrack makes sure that A and I in a track
// list never fall back to the playing track, for example on an episode or
// when a filter matches nothing.
func TestGoToInTrackListIgnoresPlayingTrack(t *testing.T) {
	m := newGoToModel(t, trackOne)
	episode := &daemon.ApiTrack{Uri: "spotify:episode:e", Name: "Episode", AlbumUri: "spotify:show:s", AlbumName: "Show"}
	p := openTracks(m, episode)

	pressKey(m, 'I')
	if m.current() != p || m.flash == "" {
		t.Errorf("I on an episode opened %T (flash %q), want a message", m.current(), m.flash)
	}
	pressKey(m, 'A')
	if got := currentTracks(t, m); got.uri != "spotify:show:s" {
		t.Errorf("A on an episode opened %q, want its show", got.uri)
	}
	m.pop()

	p.list.setFilter("nothing matches this")
	m.flash = ""
	pressKey(m, 'A')
	if m.current() != p || m.flash == "" {
		t.Errorf("A with no selection opened %T (flash %q), want a message", m.current(), m.flash)
	}
}

func TestSearchTrackRowsHaveLinks(t *testing.T) {
	p := newSearchPage()
	p.results = &webapi.SearchResult{}
	p.results.Tracks.Items = []webapi.Track{{
		URI: "spotify:track:1", Name: "Song",
		Album:   webapi.Album{URI: "spotify:album:a", Name: "Record"},
		Artists: []webapi.Artist{{URI: "spotify:artist:b", Name: "Band"}, {Name: "No URI"}},
	}}
	p.rebuild()

	r := p.list.selected()
	if r == nil {
		t.Fatal("no row selected")
	}
	if r.album != (ref{uri: "spotify:album:a", name: "Record"}) {
		t.Errorf("album = %+v", r.album)
	}
	if len(r.artists) != 1 || r.artists[0] != (ref{uri: "spotify:artist:b", name: "Band"}) {
		t.Errorf("artists = %+v", r.artists)
	}
}

func TestRefs(t *testing.T) {
	got := refs([]string{"u1", "", "u3"}, []string{"One", "Two"})
	want := []ref{{uri: "u1", name: "One"}, {uri: "u3", name: "u3"}}
	if len(got) != len(want) || got[0] != want[0] || got[1] != want[1] {
		t.Errorf("refs = %+v, want %+v", got, want)
	}
}
