package ui

import (
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"
	"github.com/charmbracelet/x/ansi"
	"github.com/devgianlu/go-librespot/daemon"

	"github.com/juthrbog/spotui/internal/webapi"
)

func labels(items []action) string {
	out := make([]string, len(items))
	for i, it := range items {
		out[i] = it.label
	}
	return strings.Join(out, ", ")
}

func TestActionsForItemTypes(t *testing.T) {
	cases := []struct {
		name string
		item row
		want string
	}{
		{"track", trackRow(trackTwo), "Add to queue, Like or unlike, Add to playlist, Go to album, Go to artist"},
		{"liked track", trackRow(trackOne), "Add to queue, Unlike, Add to playlist, Go to album, Go to artist"},
		{"album", albumRow(webapi.Album{URI: "spotify:album:a", Name: "Rec", Artists: []webapi.Artist{{URI: "spotify:artist:b", Name: "Band"}}}), "Play, Go to artist"},
		{"album without artist links", albumRow(webapi.Album{URI: "spotify:album:a", Name: "Rec"}), "Play"},
		{"episode", row{uri: "spotify:episode:e", album: ref{uri: "spotify:show:s", name: "Show"}}, "Add to queue, Add to playlist, Go to album"},
		{"track still loading", row{uri: "spotify:track:9"}, "Add to queue, Like or unlike, Add to playlist"},
	}
	liked := map[string]bool{trackOne.Uri: true}
	for _, c := range cases {
		if got := labels(actionsFor(&c.item, liked, ref{})); got != c.want {
			t.Errorf("%s: actions = %q, want %q", c.name, got, c.want)
		}
	}
}

func TestMenuRunsActionForSelectedTrack(t *testing.T) {
	m := newGoToModel(t, trackOne)
	openTracks(m, trackOne, trackTwo)
	pressKey(m, 'j')

	pressKey(m, 'a')
	if m.menu == nil || m.menu.title != "Second Song" {
		t.Fatalf("menu = %+v, want a menu for Second Song", m.menu)
	}
	frame := ansi.Strip(checkFrame(t, m, 100, 30, "menu"))
	if !strings.Contains(frame, "Second Song") {
		t.Errorf("frame does not show the title:\n%s", frame)
	}
	for _, want := range [][2]string{{"Add to queue", "e"}, {"Like or unlike", "l"}, {"Go to album", "A"}, {"Go to artist", "I"}} {
		if !menuLineHasKey(frame, want[0], want[1]) {
			t.Errorf("no menu line shows %q with its key %q:\n%s", want[0], want[1], frame)
		}
	}

	for m.menu.items[m.menu.cursor].label != "Go to album" {
		pressKey(m, 'j')
	}
	m.Update(tea.KeyPressMsg{Code: tea.KeyEnter})
	if m.menu != nil {
		t.Error("menu is still open after enter")
	}
	if got := currentTracks(t, m); got.uri != "spotify:album:a2" {
		t.Errorf("Go to album opened %q, want the album of the selected track", got.uri)
	}
}

// menuLineHasKey reports whether a line of frame shows label and then key
// as the last word in the menu box.
func menuLineHasKey(frame, label, key string) bool {
	for _, line := range strings.Split(frame, "\n") {
		i := strings.Index(line, label)
		if i < 0 {
			continue
		}
		rest := strings.Fields(strings.TrimSpace(line[i+len(label):]))
		if len(rest) >= 2 && rest[0] == key && rest[1] == "│" {
			return true
		}
	}
	return false
}

// TestMenuKeepsItsItem makes sure that the action applies to the item of
// the menu, also when the playing track changes while the menu is open.
func TestMenuKeepsItsItem(t *testing.T) {
	m := newGoToModel(t, trackOne)
	pressKey(m, 'a')
	m.Update(statusMsg{st: &daemon.ApiStatus{Username: "jakob", Track: trackTwo}})

	pressKey(m, 'A')
	if got := currentTracks(t, m); got.uri != "spotify:album:a1" {
		t.Errorf("Go to album opened %q, want the album of the track in the menu", got.uri)
	}
}

func TestGoToArtistOnOwnArtistPage(t *testing.T) {
	m := newWebModel(t)
	p := newArtistPage("spotify:artist:b", "Band")
	m.push(p)
	m.Update(artistAlbumsMsg{page: p, group: webapi.GroupAlbum, total: 1, albums: []webapi.Album{
		{URI: "spotify:album:a", Name: "Rec", Artists: []webapi.Artist{{URI: "spotify:artist:b", Name: "Band"}}},
	}})
	pressKey(m, 'G')

	pressKey(m, 'I')
	if m.current() != p || m.flash == "" {
		t.Errorf("I opened %T (flash %q), want the same page and a message", m.current(), m.flash)
	}
}

func TestMenuDirectKeyAndEsc(t *testing.T) {
	m := newGoToModel(t, nil)
	p := openTracks(m, trackTwo)

	pressKey(m, 'a')
	pressKey(m, 'n') // not an action key: must not skip to the next track
	if m.menu == nil {
		t.Fatal("an unknown key closed the menu")
	}
	m.Update(tea.KeyPressMsg{Code: tea.KeyEscape})
	if m.menu != nil || m.current() != p {
		t.Errorf("esc: menu %v, page %T; want the menu closed and the same page", m.menu, m.current())
	}

	pressKey(m, 'a')
	pressKey(m, 'I')
	if _, ok := m.current().(*artistsPage); !ok || m.menu != nil {
		t.Errorf("I in the menu opened %T (menu %v), want the artist chooser", m.current(), m.menu)
	}
}

func TestMenuForPlayingTrack(t *testing.T) {
	m := newGoToModel(t, trackOne)
	pressKey(m, 'a')
	if m.menu == nil || m.menu.title != "Now playing: First Song" {
		t.Fatalf("menu = %+v, want a menu for the playing track", m.menu)
	}
}

func TestMenuWithoutItemShowsMessage(t *testing.T) {
	m := newGoToModel(t, nil)
	pressKey(m, 'a')
	if m.menu != nil || m.flash == "" {
		t.Errorf("menu = %+v, flash = %q; want no menu and a message", m.menu, m.flash)
	}
}

func TestMenuWebActionNeedsLogin(t *testing.T) {
	m := newGoToModel(t, nil)
	ran := false
	m.menu = &actionMenu{title: "Rec", items: []action{{label: "Save album", needsWeb: true, run: func(*Model, *row) tea.Cmd {
		ran = true
		return nil
	}}}}
	if frame := ansi.Strip(checkFrame(t, m, 100, 30, "web menu")); !strings.Contains(frame, "needs L login") {
		t.Errorf("frame does not mark the Web API action:\n%s", frame)
	}
	m.Update(tea.KeyPressMsg{Code: tea.KeyEnter})
	if ran || !strings.Contains(m.flash, "L") {
		t.Errorf("ran = %v, flash = %q; want no run and a login hint", ran, m.flash)
	}
}

func TestMenuOnAlbumRowPlaysAlbum(t *testing.T) {
	m := newGoToModel(t, nil)
	m.web = newWebModel(t).web
	m.Update(albumsMsg{items: []webapi.Album{{URI: "spotify:album:a", Name: "Rec"}}})
	m.library.list.cursor = len(m.library.list.rows) - 1
	if sel := m.current().selection(); sel == nil || sel.uri != "spotify:album:a" {
		t.Fatalf("selection = %+v, want the album", sel)
	}
	pressKey(m, 'a')
	if m.menu == nil || labels(m.menu.items) != "Play" {
		t.Fatalf("menu = %+v, want Play", m.menu)
	}
}

// TestMenuFitsSmallTerminal makes sure that the menu box never makes a
// frame wider or taller than the terminal.
func TestMenuFitsSmallTerminal(t *testing.T) {
	long := &daemon.ApiTrack{
		Uri: "spotify:track:l", Name: strings.Repeat("Very Long Title ", 10),
		AlbumUri: "spotify:album:l", AlbumName: "A", ArtistUris: []string{"spotify:artist:l"}, ArtistNames: []string{"B"},
	}
	m := newGoToModel(t, long)
	pressKey(m, 'a')
	for _, size := range [][2]int{{120, 40}, {80, 24}, {40, 12}, {30, 10}} {
		m.Update(tea.WindowSizeMsg{Width: size[0], Height: size[1]})
		checkFrame(t, m, size[0], size[1], "menu")
	}
}
