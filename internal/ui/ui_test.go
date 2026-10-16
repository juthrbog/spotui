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

func TestListSkipsHeadersAndNotes(t *testing.T) {
	l := list{columns: []column{{weight: 1}}}
	l.setRows([]row{
		{kind: rowHeader, cols: []string{"A"}},
		{cols: []string{"a1"}},
		{kind: rowNote, cols: []string{"note"}},
		{kind: rowHeader, cols: []string{"B"}},
		{cols: []string{"b1"}},
	})
	if l.cursor != 1 {
		t.Fatalf("initial cursor = %d, want 1", l.cursor)
	}
	l.move(1)
	if l.cursor != 4 {
		t.Errorf("after down cursor = %d, want 4", l.cursor)
	}
	l.move(1)
	if l.cursor != 4 {
		t.Errorf("down at end moved to %d", l.cursor)
	}
	l.move(-1)
	if l.cursor != 1 {
		t.Errorf("after up cursor = %d, want 1", l.cursor)
	}
	l.move(-1)
	if l.cursor != 1 {
		t.Errorf("up at top moved to header %d", l.cursor)
	}
	l.handleKey("G")
	if l.cursor != 4 {
		t.Errorf("G cursor = %d, want 4", l.cursor)
	}
	l.handleKey("g")
	if l.cursor != 1 {
		t.Errorf("g cursor = %d, want 1", l.cursor)
	}
}

func TestLayoutColumnsFillsWidth(t *testing.T) {
	cols := []column{{width: 4}, {weight: 3}, {weight: 2}, {width: 7}}
	for _, total := range []int{20, 61, 80, 133} {
		widths := layoutColumns(cols, total)
		sum := 2 * (len(cols) - 1)
		for _, w := range widths {
			sum += w
		}
		if sum != total {
			t.Errorf("total %d: widths %v sum to %d", total, widths, sum)
		}
	}
}

func TestFormatMs(t *testing.T) {
	cases := map[int64]string{0: "0:00", 61_000: "1:01", 3_725_000: "1:02:05", -5: "0:00"}
	for in, want := range cases {
		if got := formatMs(in); got != want {
			t.Errorf("formatMs(%d) = %q, want %q", in, got, want)
		}
	}
}

func TestFitPadsAndTruncates(t *testing.T) {
	if got := fit("abc", 5, false); got != "abc  " {
		t.Errorf("pad = %q", got)
	}
	if got := fit("abc", 5, true); got != "  abc" {
		t.Errorf("right = %q", got)
	}
	if got := fit("abcdef", 4, false); ansi.StringWidth(got) != 4 || !strings.HasSuffix(got, "…") {
		t.Errorf("truncate = %q", got)
	}
}

// TestRenderFitsTerminal feeds the model the messages of a normal session and
// checks that every frame fills the terminal exactly, without wrapping.
func TestRenderFitsTerminal(t *testing.T) {
	var opened []string
	openBrowser = func(url string) { opened = append(opened, url) }
	web := webapi.New("", 8898, filepath.Join(t.TempDir(), "token.json"))
	m := New(context.Background(), engine.New(engine.Options{}), web, Info{
		ConfigFile:  "/home/user/.config/spotui/config.toml",
		RedirectURL: "http://127.0.0.1:8898/callback",
	})

	track := &daemon.ApiTrack{
		Uri: "spotify:track:2", Name: "A Very Long Song Title That Goes On And On", AlbumName: "Album",
		ArtistNames: []string{"Artist One", "Artist Two"}, Duration: 245_000, Position: 83_000,
	}
	status := &daemon.ApiStatus{Username: "jakob", Track: track, Volume: 60, VolumeSteps: 100, ShuffleContext: true}

	for _, size := range [][2]int{{120, 40}, {80, 24}, {40, 12}} {
		w, h := size[0], size[1]
		opened = nil
		m.Update(tea.WindowSizeMsg{Width: w, Height: h})
		checkFrame(t, m, w, h, "connecting")

		m.Update(engineEventMsg{ev: engine.AuthCode{URL: "https://accounts.spotify.com/pair?code=ABCD", Code: "ABCD"}})
		checkFrame(t, m, w, h, "device login")
		if len(opened) != 1 {
			t.Errorf("browser opened %d times, want once", len(opened))
		}

		m.Update(statusMsg{st: status, poll: false})
		m.Update(playlistsMsg{items: []daemon.ApiLibraryPlaylist{
			{Name: "Road Trip", Uri: "spotify:playlist:1", Length: 42, OwnerUsername: "jakob"},
			{Name: "Someone Else's Mix", Uri: "spotify:playlist:2", Length: 1, OwnerUsername: "friend", Folder: []string{"Shared"}},
		}})
		checkFrame(t, m, w, h, "library")

		m.Update(tea.KeyPressMsg{Code: tea.KeyEnter})
		page, ok := m.current().(*tracksPage)
		if !ok {
			t.Fatalf("enter on Liked Songs opened %T", m.current())
		}
		m.Update(contextTracksMsg{page: page, resp: &daemon.ApiContextTracks{
			Ready: true, Length: 2, Cached: 1,
			Tracks: []daemon.ApiContextTrackItem{{Uri: "spotify:track:1"}, {Uri: track.Uri, Track: track}},
		}})
		frame := checkFrame(t, m, w, h, "tracks")
		if !strings.Contains(frame, "▶") {
			t.Errorf("%dx%d tracks: playing track has no marker", w, h)
		}

		m.Update(tea.KeyPressMsg{Code: tea.KeyEscape})
		if _, ok := m.current().(*libraryPage); !ok {
			t.Fatalf("esc did not go back, current page is %T", m.current())
		}

		m.Update(tea.KeyPressMsg{Code: '2', Text: "2"})
		checkFrame(t, m, w, h, "search setup")

		m.Update(tea.KeyPressMsg{Code: '?', Text: "?"})
		checkFrame(t, m, w, h, "help")
		m.Update(tea.KeyPressMsg{Code: 'x', Text: "x"})
		m.Update(tea.KeyPressMsg{Code: '1', Text: "1"})
	}
}

// TestSearchInputCanAlwaysBeLeft makes sure that the search box never traps
// the keyboard, also before the first search returns results.
func TestSearchInputCanAlwaysBeLeft(t *testing.T) {
	web := webapi.New("client-id", 8898, filepath.Join(t.TempDir(), "token.json"))
	m := New(context.Background(), engine.New(engine.Options{}), web, Info{})
	m.Update(tea.WindowSizeMsg{Width: 100, Height: 30})
	m.webLoginTried = true // keep the test from starting a Web API login
	m.Update(statusMsg{st: &daemon.ApiStatus{Username: "jakob"}})

	typeKey := func(code rune, text string) { m.Update(tea.KeyPressMsg{Code: code, Text: text}) }

	typeKey('/', "/")
	if !m.current().typing() {
		t.Fatal("/ did not focus the search box")
	}
	if footer := ansi.Strip(m.footerView(100)); !strings.Contains(footer, "esc") {
		t.Errorf("footer while typing does not name esc: %q", footer)
	}
	typeKey('a', "a")
	if got := m.search.input.Value(); got != "a" {
		t.Fatalf("search box = %q, want a", got)
	}

	m.Update(tea.KeyPressMsg{Code: tea.KeyEscape})
	if m.current().typing() {
		t.Fatal("esc did not leave the search box before any results")
	}
	typeKey('n', "n")
	if got := m.search.input.Value(); got != "a" {
		t.Errorf("a key went into the search box after esc: %q", got)
	}

	typeKey('/', "/")
	m.Update(tea.KeyPressMsg{Code: tea.KeyTab})
	if m.search.typing() {
		t.Error("tab did not leave the search box")
	}
	if m.tab != tabLibrary {
		t.Errorf("tab did not switch to the library, tab = %d", m.tab)
	}
}

func checkFrame(t *testing.T, m *Model, w, h int, name string) string {
	t.Helper()
	frame := m.View().Content
	lines := strings.Split(frame, "\n")
	if len(lines) != h {
		t.Errorf("%dx%d %s: %d lines, want %d", w, h, name, len(lines), h)
	}
	for i, l := range lines {
		if lw := ansi.StringWidth(l); lw > w {
			t.Errorf("%dx%d %s: line %d is %d cells wide: %q", w, h, name, i, lw, ansi.Strip(l))
		}
	}
	return frame
}

func TestBrowserCommand(t *testing.T) {
	url := "https://accounts.spotify.com/authorize?a=1&b=2"
	cases := map[string][]string{
		"linux":   {"xdg-open", url},
		"darwin":  {"open", url},
		"windows": {"rundll32", "url.dll,FileProtocolHandler", url},
	}
	for goos, want := range cases {
		name, args := browserCommand(goos, url)
		if got := append([]string{name}, args...); strings.Join(got, " ") != strings.Join(want, " ") {
			t.Errorf("%s: %q, want %q", goos, got, want)
		}
	}
}
