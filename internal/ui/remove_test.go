package ui

import (
	"fmt"
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"
	"github.com/charmbracelet/x/ansi"
	"github.com/devgianlu/go-librespot/daemon"
	"github.com/devgianlu/go-librespot/spclient"
)

// openPlaylist opens the playlist with the URI uri as a track list with
// trackOne and trackTwo.
func openPlaylist(m *Model, uri string) *tracksPage {
	p := newTracksPage(uri, "List")
	m.push(p)
	m.Update(contextTracksMsg{page: p, resp: &daemon.ApiContextTracks{
		Ready: true, Length: 2, Cached: 2,
		Tracks: []daemon.ApiContextTrackItem{{Uri: trackOne.Uri, Track: trackOne}, {Uri: trackTwo.Uri, Track: trackTwo}},
	}})
	return p
}

func menuLabels(t *testing.T, m *Model) string {
	t.Helper()
	pressKey(m, 'a')
	if m.menu == nil {
		t.Fatal("no menu")
	}
	got := labels(m.menu.items)
	m.Update(tea.KeyPressMsg{Code: tea.KeyEscape})
	return got
}

func TestRemoveShowsOnlyInPlaylistThatCanChange(t *testing.T) {
	m := newGoToModel(t, nil)
	m.Update(playlistsMsg{items: editablePlaylists})

	openPlaylist(m, "spotify:playlist:mine")
	if got := menuLabels(t, m); !strings.Contains(got, "Remove from this playlist") {
		t.Errorf("own playlist: %s", got)
	}
	m.pop()
	openPlaylist(m, "spotify:playlist:theirs")
	if got := menuLabels(t, m); strings.Contains(got, "Remove") {
		t.Errorf("playlist of someone else: %s", got)
	}
	m.pop()
	openPlaylist(m, "spotify:album:a1")
	if got := menuLabels(t, m); strings.Contains(got, "Remove") {
		t.Errorf("album: %s", got)
	}
}

func TestRemoveAsksAndKeepsPositionWithFilter(t *testing.T) {
	m := newGoToModel(t, nil)
	m.Update(playlistsMsg{items: editablePlaylists})
	p := openPlaylist(m, "spotify:playlist:mine")
	p.list.setFilter("second")
	if sel := p.list.selected(); sel == nil || sel.pos != 1 {
		t.Fatalf("filtered selection = %+v, want position 1", sel)
	}

	pressKey(m, 'a')
	for i, it := range m.menu.items {
		if it.label == "Remove from this playlist" {
			m.menu.cursor = i
		}
	}
	m.Update(tea.KeyPressMsg{Code: tea.KeyEnter})
	if m.confirm == nil || m.confirm.question != "Remove Second Song from Mine?" {
		t.Fatalf("confirm = %+v", m.confirm)
	}
	if footer := ansi.Strip(m.footerView(100)); !strings.Contains(footer, "Remove Second Song from Mine? (y/n)") {
		t.Errorf("footer = %q", footer)
	}

	pressKey(m, 'n')
	if m.confirm != nil {
		t.Fatal("n did not cancel")
	}

	m.confirmRemove(p.list.selected(), ref{uri: "spotify:playlist:mine", name: "Mine"})
	pressKey(m, 'q') // other keys do nothing
	if m.confirm == nil {
		t.Fatal("another key closed the question")
	}
	if cmd := m.handleKey(tea.KeyPressMsg{Code: 'y', Text: "y"}); cmd == nil || m.confirm != nil {
		t.Errorf("y: cmd = %v, confirm = %+v", cmd, m.confirm)
	}
}

// TestRemovePositionOnlyForCopies makes sure that a remove names the
// position only when the list has the item more than one time. Without a
// local file before the item, the position matches the playlist.
func TestRemovePositionOnlyForCopies(t *testing.T) {
	m := newGoToModel(t, nil)
	p := newTracksPage("spotify:playlist:mine", "Mine")
	m.push(p)
	m.Update(contextTracksMsg{page: p, resp: &daemon.ApiContextTracks{
		Ready: true, Length: 3, Cached: 3,
		Tracks: []daemon.ApiContextTrackItem{
			{Uri: trackOne.Uri, Track: trackOne}, {Uri: trackTwo.Uri, Track: trackTwo}, {Uri: trackOne.Uri, Track: trackOne},
		},
	}})

	if got := m.removePosition(&p.list.rows[1]); got != nil {
		t.Errorf("single copy: position %d, want none", *got)
	}
	if got := m.removePosition(&p.list.rows[2]); got == nil || *got != 2 {
		t.Errorf("second of two copies: position %v, want 2", got)
	}
}

func TestFlashWaitsForQuestion(t *testing.T) {
	m := newGoToModel(t, nil)
	m.setFlash("Could not add to Mine: boom", true)
	m.confirm = &confirmPrompt{question: "Remove?", yes: func() tea.Cmd { return nil }}

	if cmd := m.update(flashClearMsg{id: m.flashID}); cmd == nil || m.flash == "" {
		t.Fatalf("the message was cleared while the question hid it")
	}
	pressKey(m, 'n')
	if footer := ansi.Strip(m.footerView(100)); !strings.Contains(footer, "boom") {
		t.Errorf("footer after the question = %q", footer)
	}
	m.update(flashClearMsg{id: m.flashID})
	if m.flash != "" {
		t.Error("the message stayed after the question closed")
	}
}

func TestRemoveResult(t *testing.T) {
	m := newGoToModel(t, nil)
	m.Update(playlistsMsg{items: []daemon.ApiLibraryPlaylist{{Name: "Mine", Uri: "spotify:playlist:mine", CanEdit: true, Length: 12}}})
	p := openPlaylist(m, "spotify:playlist:mine")

	m.update(playlistChangedMsg{playlistURI: p.uri, playlist: "Mine", track: "Second Song", removed: true})
	if m.flash != "Removed Second Song from Mine." || m.flashErr {
		t.Errorf("flash = %q", m.flash)
	}
	if got := m.library.playlists[0].Length; got != 11 {
		t.Errorf("Length = %d, want 11", got)
	}
	if p.gen != 1 {
		t.Error("the page did not load again after the remove")
	}

	conflict := fmt.Errorf("failed removing from playlist: %w", spclient.ErrPlaylistConflict)
	m.update(playlistChangedMsg{playlistURI: p.uri, playlist: "Mine", track: "Second Song", removed: true, err: conflict})
	if !strings.Contains(m.flash, "nothing was removed") || !m.flashErr {
		t.Errorf("conflict flash = %q", m.flash)
	}
	// go-librespot keeps its old copy after a failed remove, so a reload
	// would only show the same list.
	if p.gen != 1 {
		t.Error("the page loaded again after a conflict")
	}

	notFound := fmt.Errorf("x: %w", daemon.ErrNotFound)
	m.update(playlistChangedMsg{playlistURI: p.uri, playlist: "Mine", track: "Second Song", removed: true, err: notFound})
	if m.flash != "Second Song is no longer in Mine." {
		t.Errorf("not found flash = %q", m.flash)
	}
	if got := m.library.playlists[0].Length; got != 11 {
		t.Errorf("a conflict changed Length to %d", got)
	}

	forbidden := fmt.Errorf("x: %w", daemon.ErrForbidden)
	m.update(playlistChangedMsg{playlistURI: p.uri, playlist: "Mine", removed: true, err: forbidden})
	if m.flash != "You cannot change the playlist Mine." {
		t.Errorf("forbidden flash = %q", m.flash)
	}
}
