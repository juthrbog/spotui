package ui

import (
	"errors"
	"fmt"
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"
	"github.com/devgianlu/go-librespot/daemon"
)

var editablePlaylists = []daemon.ApiLibraryPlaylist{
	{Name: "Mine", Uri: "spotify:playlist:mine", CanEdit: true, OwnerUsername: "jakob"},
	{Name: "Theirs", Uri: "spotify:playlist:theirs", OwnerUsername: "friend"},
	{Name: "Shared", Uri: "spotify:playlist:shared", CanEdit: true, Collaborative: true, OwnerUsername: "friend", Folder: []string{"Party"}},
}

func openPicker(t *testing.T, m *Model) *playlistPicker {
	t.Helper()
	pressKey(m, 'a')
	for i, it := range m.menu.items {
		if it.label == "Add to playlist" {
			m.menu.cursor = i
		}
	}
	m.Update(tea.KeyPressMsg{Code: tea.KeyEnter})
	p, ok := m.current().(*playlistPicker)
	if !ok {
		t.Fatalf("Add to playlist opened %T", m.current())
	}
	return p
}

func TestPickerShowsOnlyPlaylistsThatCanChange(t *testing.T) {
	m := newGoToModel(t, nil)
	m.Update(playlistsMsg{items: editablePlaylists})
	openTracks(m, trackTwo)

	p := openPicker(t, m)
	if p.title() != "Add Second Song to" {
		t.Errorf("title = %q", p.title())
	}
	text := strings.Join(rowTexts(&p.list), "\n")
	if strings.Contains(text, "Theirs") || !strings.Contains(text, "Mine") || !strings.Contains(text, "Party / Shared") {
		t.Errorf("picker rows:\n%s", text)
	}
	checkFrame(t, m, 100, 30, "picker")

	// The filter works in the picker.
	pressKey(m, 'f')
	for _, r := range "party" {
		pressKey(m, r)
	}
	m.Update(tea.KeyPressMsg{Code: tea.KeyEnter})
	if sel := p.list.selected(); sel == nil || sel.uri != "spotify:playlist:shared" {
		t.Fatalf("filter selected %+v", sel)
	}

	cmd := p.update(m, tea.KeyPressMsg{Code: tea.KeyEnter})
	if cmd == nil {
		t.Error("enter sent nothing")
	}
	if _, ok := m.current().(*tracksPage); !ok {
		t.Errorf("enter did not close the picker, current page is %T", m.current())
	}
}

func TestPickerWaitsForPlaylists(t *testing.T) {
	m := newGoToModel(t, nil)
	m.library.playlistsLoading = true
	p := newPlaylistPicker(m, row{uri: trackOne.Uri, label: "First Song"})
	m.push(p)
	if text := strings.Join(rowTexts(&p.list), "\n"); !strings.Contains(text, "Loading playlists") {
		t.Errorf("rows while loading:\n%s", text)
	}
	m.Update(playlistsMsg{items: editablePlaylists})
	if text := strings.Join(rowTexts(&p.list), "\n"); !strings.Contains(text, "Mine") {
		t.Errorf("rows after load:\n%s", text)
	}
}

func TestPickerWithoutPlaylistsThatCanChange(t *testing.T) {
	m := newGoToModel(t, nil)
	m.Update(playlistsMsg{items: editablePlaylists[1:2]})
	p := newPlaylistPicker(m, row{uri: trackOne.Uri})
	if text := strings.Join(rowTexts(&p.list), "\n"); !strings.Contains(text, "No playlists that you can change") {
		t.Errorf("rows:\n%s", text)
	}
}

func TestPickerBeforePlayerIsReady(t *testing.T) {
	m := newGoToModel(t, nil)
	m.ready = false
	p := newPlaylistPicker(m, row{uri: trackOne.Uri})
	if text := strings.Join(rowTexts(&p.list), "\n"); !strings.Contains(text, "Loading playlists") {
		t.Errorf("rows before the player is ready:\n%s", text)
	}
}

func TestPlaylistAddCountsTheNewTrack(t *testing.T) {
	m := newGoToModel(t, nil)
	m.Update(playlistsMsg{items: []daemon.ApiLibraryPlaylist{{Name: "Mine", Uri: "spotify:playlist:mine", CanEdit: true, Length: 12}}})
	m.update(playlistAddMsg{playlistURI: "spotify:playlist:mine", playlist: "Mine", track: "Song"})
	if got := m.library.playlists[0].Length; got != 13 {
		t.Errorf("Length = %d, want 13", got)
	}
	if text := strings.Join(rowTexts(&m.library.list), "\n"); !strings.Contains(text, "13 tracks") {
		t.Errorf("library rows:\n%s", text)
	}
	m.update(playlistAddMsg{playlistURI: "spotify:playlist:mine", playlist: "Mine", err: errors.New("boom")})
	if got := m.library.playlists[0].Length; got != 13 {
		t.Errorf("a failed add changed Length to %d", got)
	}
}

func TestPlaylistAddResult(t *testing.T) {
	m := newGoToModel(t, nil)
	page := newTracksPage("spotify:playlist:mine", "Mine")
	m.push(page)
	m.Update(contextTracksMsg{page: page, resp: &daemon.ApiContextTracks{Ready: true}})

	m.update(playlistAddMsg{playlistURI: "spotify:playlist:mine", playlist: "Mine", track: "First Song"})
	if m.flash != "Added First Song to Mine." || m.flashErr {
		t.Errorf("flash = %q", m.flash)
	}
	if page.gen != 1 {
		t.Error("the open playlist page did not load again")
	}

	forbidden := fmt.Errorf("failed appending to playlist: %w: %w", daemon.ErrForbidden, errors.New("403"))
	m.update(playlistAddMsg{playlistURI: "spotify:playlist:mine", playlist: "Mine", track: "First Song", err: forbidden})
	if m.flash != "You cannot change the playlist Mine." || !m.flashErr {
		t.Errorf("forbidden flash = %q", m.flash)
	}
	if page.gen != 1 {
		t.Error("a failed add loaded the page again")
	}

	m.update(playlistAddMsg{playlistURI: "spotify:playlist:mine", playlist: "Mine", track: "First Song", err: errors.New("boom")})
	if !strings.Contains(m.flash, "boom") || !m.flashErr {
		t.Errorf("error flash = %q", m.flash)
	}
}

func TestAddToPlaylistOnlyForSongsAndEpisodes(t *testing.T) {
	m := newGoToModel(t, nil)
	m.openPlaylistPicker(&row{uri: "spotify:local:a:b:c:1"})
	if _, ok := m.current().(*libraryPage); !ok || !m.flashErr {
		t.Errorf("local file opened %T, flash %q", m.current(), m.flash)
	}
}
