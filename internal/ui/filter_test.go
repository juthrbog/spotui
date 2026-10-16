package ui

import (
	"context"
	"path/filepath"
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"
	"github.com/charmbracelet/x/ansi"
	"github.com/devgianlu/go-librespot/daemon"

	"github.com/juthrbog/spotui/internal/engine"
	"github.com/juthrbog/spotui/internal/webapi"
)

func item(uri string, info ...string) row {
	return row{cols: []string{info[0]}, uri: uri, info: info}
}

func visible(l *list) []string {
	var out []string
	for _, r := range l.rows {
		switch r.kind {
		case rowItem:
			out = append(out, r.uri)
		case rowHeader:
			if len(r.cols) > 0 {
				out = append(out, "#"+r.cols[0])
			}
		case rowNote:
			out = append(out, "!"+r.cols[0])
		}
	}
	return out
}

func TestFilterRows(t *testing.T) {
	l := list{columns: []column{{weight: 1}}}
	l.setRows([]row{
		{kind: rowHeader, cols: []string{"Playlists"}},
		item("p1", "Road Trip", "Playlist"),
		item("p2", "Chill Mix", "Playlist in Moods", "by spotify"),
		note("a note"),
		{kind: rowHeader},
		{kind: rowHeader, cols: []string{"Albums"}},
		item("a1", "Abbey Road", "The Beatles", "Album · 1969"),
	})

	cases := []struct {
		query string
		want  string
	}{
		{"", "#Playlists p1 p2 !a note #Albums a1"},
		{"road", "#Playlists p1 #Albums a1"},
		{"ROAD beatles", "#Albums a1"},
		{"moods", "#Playlists p2"},
		{"1969", "#Albums a1"},
		{"nothing here", "!No matches."},
	}
	for _, c := range cases {
		l.setFilter(c.query)
		if got := strings.Join(visible(&l), " "); got != c.want {
			t.Errorf("filter %q = %q, want %q", c.query, got, c.want)
		}
	}
}

func TestFilterKeepsSelection(t *testing.T) {
	l := list{columns: []column{{weight: 1}}}
	l.setRows([]row{item("a", "Alpha"), item("b", "Beta"), item("c", "Gamma")})
	l.move(2) // Gamma

	l.setFilter("a") // every name has an "a"
	if sel := l.selected(); sel == nil || sel.uri != "c" {
		t.Fatalf("selection after a matching filter = %+v, want c", sel)
	}
	l.setFilter("beta")
	if sel := l.selected(); sel == nil || sel.uri != "b" {
		t.Fatalf("selection after a filter that hides it = %+v, want the first match b", sel)
	}
	l.setFilter("")
	if sel := l.selected(); sel == nil || sel.uri != "b" {
		t.Errorf("selection after clearing = %+v, want b", sel)
	}
	if shown, total := l.counts(); shown != 3 || total != 3 {
		t.Errorf("counts = %d of %d, want 3 of 3", shown, total)
	}
}

func TestReloadKeepsPositionWithDuplicates(t *testing.T) {
	// A playlist can hold the same track twice. A reload while the track
	// list fills in must not move the cursor to the first copy.
	rows := []row{item("x", "Song"), item("y", "Other"), item("x", "Song")}
	l := list{columns: []column{{weight: 1}}}
	l.setRows(rows)
	l.move(2)
	l.setRows(rows)
	if l.cursor != 2 {
		t.Errorf("cursor after reload = %d, want 2", l.cursor)
	}
}

func filterModel(t *testing.T) *Model {
	t.Helper()
	openBrowser = func(string) {}
	web := webapi.New("", 8898, filepath.Join(t.TempDir(), "token.json"))
	m := New(context.Background(), engine.New(engine.Options{}), web, Info{CoverArt: "off"})
	m.Update(tea.WindowSizeMsg{Width: 100, Height: 30})
	m.Update(statusMsg{st: &daemon.ApiStatus{Username: "jakob"}})
	m.Update(playlistsMsg{items: []daemon.ApiLibraryPlaylist{
		{Name: "Road Trip", Uri: "spotify:playlist:road", Length: 2},
		{Name: "Chill Mix", Uri: "spotify:playlist:chill", Length: 1, Folder: []string{"Moods"}},
	}})
	return m
}

func typeText(m *Model, s string) {
	for _, r := range s {
		m.Update(tea.KeyPressMsg{Code: r, Text: string(r)})
	}
}

func key(m *Model, code rune) { m.Update(tea.KeyPressMsg{Code: code}) }

func TestLibraryFilterFlow(t *testing.T) {
	m := filterModel(t)

	typeText(m, "f")
	if !m.current().typing() {
		t.Fatal("f did not open the filter")
	}
	if footer := ansi.Strip(m.footerView(100)); !strings.Contains(footer, "typing a filter") {
		t.Errorf("footer while filtering = %q", footer)
	}

	// Player keys go into the box while typing.
	typeText(m, "road")
	if got := strings.Join(visible(&m.library.list), " "); got != "#Playlists spotify:playlist:road" {
		t.Fatalf("filtered library = %q", got)
	}
	frame := checkFrame(t, m, 100, 30, "library filter")
	if !strings.Contains(ansi.Strip(frame), "1 of 3") {
		t.Error("filter line does not show the count")
	}

	key(m, tea.KeyEnter)
	if m.current().typing() {
		t.Fatal("enter did not leave the filter box")
	}
	if m.library.filter.input.Value() != "road" {
		t.Fatal("enter removed the filter")
	}

	key(m, tea.KeyEnter)
	page, ok := m.current().(*tracksPage)
	if !ok || page.uri != "spotify:playlist:road" {
		t.Fatalf("enter opened %T, want the Road Trip tracks", m.current())
	}

	key(m, tea.KeyEscape)
	if _, ok := m.current().(*libraryPage); !ok {
		t.Fatal("esc on an unfiltered track list did not go back")
	}
	key(m, tea.KeyEscape)
	if m.library.filter.active() || len(m.library.list.rows) != len(m.library.list.all) {
		t.Error("esc did not clear the library filter")
	}
	key(m, tea.KeyEscape) // nothing left to clear or close
	if _, ok := m.current().(*libraryPage); !ok {
		t.Error("esc on the bare library changed the page")
	}
}

func TestFilterEscWhileTypingClears(t *testing.T) {
	m := filterModel(t)
	typeText(m, "fmoods")
	if sel := m.current().selection(); sel == nil || sel.uri != "spotify:playlist:chill" {
		t.Fatalf("folder name did not match, selection = %+v", sel)
	}
	key(m, tea.KeyEscape)
	if m.current().typing() || m.library.filter.active() {
		t.Error("esc while typing did not clear and close the filter")
	}
}

func TestTabLeavesFilterAndKeepsIt(t *testing.T) {
	m := filterModel(t)
	typeText(m, "fchill")
	key(m, tea.KeyTab)
	if m.tab != tabSearch {
		t.Fatalf("tab = %d, want search", m.tab)
	}
	if m.library.typing() {
		t.Error("library still typing after tab")
	}
	if m.library.filter.input.Value() != "chill" {
		t.Error("tab removed the filter")
	}
}

func TestTrackFilterSelectsMatchingTrack(t *testing.T) {
	m := filterModel(t)
	key(m, tea.KeyEnter) // Liked Songs
	page := m.current().(*tracksPage)
	m.Update(contextTracksMsg{page: page, resp: &daemon.ApiContextTracks{
		Ready: true, Length: 3, Cached: 3,
		Tracks: []daemon.ApiContextTrackItem{
			{Uri: "spotify:track:1", Track: &daemon.ApiTrack{Name: "One", ArtistNames: []string{"Metallica"}, AlbumName: "Justice"}},
			{Uri: "spotify:track:2", Track: &daemon.ApiTrack{Name: "Two Weeks", ArtistNames: []string{"Grizzly Bear"}, AlbumName: "Veckatimest"}},
			{Uri: "spotify:track:3", Track: &daemon.ApiTrack{Name: "Three", ArtistNames: []string{"Wallows"}, AlbumName: "Nothing Happens"}},
		},
	}})

	typeText(m, "fgrizzly")
	if sel := m.current().selection(); sel == nil || sel.uri != "spotify:track:2" {
		t.Fatalf("selection = %+v, want track 2", sel)
	}
	checkFrame(t, m, 100, 30, "track filter")

	// The track number is not part of the match.
	m.Update(tea.KeyPressMsg{Code: tea.KeyBackspace})
	for range len("grizzly") - 1 {
		m.Update(tea.KeyPressMsg{Code: tea.KeyBackspace})
	}
	typeText(m, "3")
	if got := visible(&page.list); len(got) != 1 || got[0] != "!No matches." {
		t.Errorf("filter 3 = %v, want no matches", got)
	}
}
