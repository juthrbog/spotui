package ui

import (
	"errors"
	"fmt"
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"
	"github.com/charmbracelet/x/ansi"
	"github.com/devgianlu/go-librespot/daemon"

	"github.com/juthrbog/spotui/internal/engine"
)

func TestLikedBatches(t *testing.T) {
	m := newGoToModel(t, nil)
	uris := []string{"spotify:episode:e", "spotify:local:a:b:c:1"}
	for i := range 120 {
		uris = append(uris, fmt.Sprintf("spotify:track:%d", i))
	}

	batches := m.likedBatches(uris)
	if len(batches) != 3 || len(batches[0]) != 50 || len(batches[1]) != 50 || len(batches[2]) != 20 {
		t.Fatalf("batch sizes = %d batches, want 50, 50, 20", len(batches))
	}
	for _, b := range batches {
		for _, uri := range b {
			if !isTrackURI(uri) {
				t.Errorf("batch has %q, which is not a track", uri)
			}
		}
	}
	if again := m.likedBatches(uris); len(again) != 0 {
		t.Errorf("asked again for %d batches", len(again))
	}
}

func TestLikedStatesShowHearts(t *testing.T) {
	m := newGoToModel(t, trackOne)
	p := openTracks(m, trackOne, trackTwo)
	m.Update(likedStatesMsg{asked: []string{trackOne.Uri, trackTwo.Uri}, states: map[string]bool{trackOne.Uri: true, trackTwo.Uri: false}})

	if got := p.list.rows[0].cols[1]; got != "♥" {
		t.Errorf("liked track has heart column %q", got)
	}
	if got := p.list.rows[1].cols[1]; got != "" {
		t.Errorf("track that is not liked has heart column %q", got)
	}
	frame := ansi.Strip(checkFrame(t, m, 100, 30, "hearts"))
	if !strings.Contains(frame, "First Song ♥") {
		t.Errorf("now playing bar has no heart:\n%s", frame)
	}
}

func TestLikedStatesErrorAsksAgain(t *testing.T) {
	m := newGoToModel(t, nil)
	m.likedBatches([]string{trackOne.Uri})
	m.Update(likedStatesMsg{asked: []string{trackOne.Uri}, err: errors.New("boom")})
	if m.likedAsked[trackOne.Uri] {
		t.Error("a failed request still counts as asked")
	}
	if m.flash != "" {
		t.Errorf("a background error showed %q", m.flash)
	}
}

func TestLikedSongsNeedsNoRequestAndShowsNoHearts(t *testing.T) {
	m := newGoToModel(t, nil)
	p := newTracksPage(engine.LikedSongsURI("jakob"), "Liked Songs")
	m.push(p)
	m.Update(contextTracksMsg{page: p, resp: &daemon.ApiContextTracks{
		Ready: true, Length: 1, Cached: 1, Tracks: []daemon.ApiContextTrackItem{{Uri: trackOne.Uri, Track: trackOne}},
	}})

	if !m.liked[trackOne.Uri] || !m.likedAsked[trackOne.Uri] {
		t.Error("tracks of Liked Songs are not marked as liked")
	}
	if got := p.list.rows[0].cols[1]; got != "" {
		t.Errorf("Liked Songs shows heart column %q", got)
	}
}

func TestLikedSetUpdatesStateAndPages(t *testing.T) {
	m := newGoToModel(t, nil)
	p := openTracks(m, trackTwo)
	liked := newTracksPage(engine.LikedSongsURI("jakob"), "Liked Songs")
	m.stacks[tabSearch] = append(m.stacks[tabSearch], liked)

	cmd := m.update(likedSetMsg{uri: trackTwo.Uri, name: "Second Song", liked: true})
	if !m.liked[trackTwo.Uri] || m.flash != "Added to Liked Songs: Second Song" || m.flashErr {
		t.Errorf("liked = %v, flash = %q", m.liked[trackTwo.Uri], m.flash)
	}
	if got := p.list.rows[0].cols[1]; got != "♥" {
		t.Errorf("track list did not show the new heart, column = %q", got)
	}
	if cmd == nil {
		t.Error("the open Liked Songs page did not load again")
	}

	m.Update(likedSetMsg{uri: trackTwo.Uri, name: "Second Song", liked: false})
	if m.liked[trackTwo.Uri] || !strings.HasPrefix(m.flash, "Removed from Liked Songs") {
		t.Errorf("after unlike: liked = %v, flash = %q", m.liked[trackTwo.Uri], m.flash)
	}

	m.Update(likedSetMsg{uri: trackTwo.Uri, name: "Second Song", liked: true, err: errors.New("boom")})
	if m.liked[trackTwo.Uri] || !m.flashErr || !strings.Contains(m.flash, "boom") {
		t.Errorf("after error: liked = %v, flash = %q", m.liked[trackTwo.Uri], m.flash)
	}
}

// TestLikedSongsReloadKeepsPlace makes sure that the reload after a like
// keeps the old rows and the cursor until the new list is ready, and that
// replies of the old load do not start a second poll.
func TestLikedSongsReloadKeepsPlace(t *testing.T) {
	m := newGoToModel(t, nil)
	p := newTracksPage(engine.LikedSongsURI("jakob"), "Liked Songs")
	m.push(p)
	three := []daemon.ApiContextTrackItem{
		{Uri: trackOne.Uri, Track: trackOne},
		{Uri: trackTwo.Uri, Track: trackTwo},
		{Uri: "spotify:track:3", Track: &daemon.ApiTrack{Uri: "spotify:track:3", Name: "Third Song"}},
	}
	m.Update(contextTracksMsg{page: p, resp: &daemon.ApiContextTracks{Ready: true, Length: 3, Cached: 2, Tracks: three}})
	p.list.handleKey("G")

	if cmd := m.update(likedSetMsg{uri: trackTwo.Uri, name: "Second Song"}); cmd == nil || !p.reloading || p.gen != 1 {
		t.Fatalf("unlike did not start a reload: gen %d, reloading %v", p.gen, p.reloading)
	}
	if cmd := p.update(m, contextTracksMsg{page: p, gen: 0, resp: &daemon.ApiContextTracks{Ready: true, Length: 3, Cached: 2, Tracks: three}}); cmd != nil {
		t.Error("a reply of the old load started another poll")
	}

	p.update(m, contextTracksMsg{page: p, gen: 1, resp: &daemon.ApiContextTracks{}})
	if len(p.list.rows) != 3 || p.list.cursor != 2 {
		t.Errorf("not-ready reply changed the list: %d rows, cursor %d", len(p.list.rows), p.list.cursor)
	}

	p.update(m, contextTracksMsg{page: p, gen: 1, resp: &daemon.ApiContextTracks{
		Ready: true, Length: 2, Cached: 2, Tracks: []daemon.ApiContextTrackItem{three[0], three[2]},
	}})
	if p.reloading || len(p.list.rows) != 2 {
		t.Fatalf("ready reply: reloading %v, %d rows", p.reloading, len(p.list.rows))
	}
	if sel := p.list.selected(); sel == nil || sel.uri != "spotify:track:3" {
		t.Errorf("cursor moved to %+v, want the third song", sel)
	}
}

func TestLikeWhileChangeIsPending(t *testing.T) {
	m := newGoToModel(t, nil)
	item := &row{uri: trackOne.Uri, label: "First Song"}
	if cmd := m.toggleLikeOf(item); cmd == nil {
		t.Fatal("first press sent nothing")
	}
	if m.toggleLikeOf(item); !strings.Contains(m.flash, "Still changing") || !m.flashErr {
		t.Errorf("second press: flash = %q", m.flash)
	}
	m.update(likedSetMsg{uri: trackOne.Uri, name: "First Song", liked: true})
	m.flash = ""
	if m.toggleLikeOf(item); m.flash != "" || !m.likePending[trackOne.Uri] {
		t.Errorf("press after the reply: flash = %q, pending = %v", m.flash, m.likePending[trackOne.Uri])
	}
}

func TestTrackName(t *testing.T) {
	if got := trackName(&row{uri: "spotify:track:9"}); got != "the track" {
		t.Errorf("loading row: %q", got)
	}
	if got := trackName(&row{uri: "spotify:track:1", label: "Song"}); got != "Song" {
		t.Errorf("row with label: %q", got)
	}
}

func TestToggleLikeChecksItem(t *testing.T) {
	m := newGoToModel(t, nil)
	if m.toggleLikeOf(nil); m.flash == "" {
		t.Error("no message without an item")
	}
	m.flash = ""
	if m.toggleLikeOf(&row{uri: "spotify:episode:e"}); !strings.Contains(m.flash, "Only songs") {
		t.Errorf("episode: flash = %q", m.flash)
	}
	m.flash = ""
	if cmd := m.toggleLikeOf(&row{uri: trackOne.Uri, label: "First Song"}); cmd == nil || m.flash != "" {
		t.Errorf("track: cmd = %v, flash = %q", cmd, m.flash)
	}
}

func TestLikeKeyInTrackList(t *testing.T) {
	m := newGoToModel(t, nil)
	openTracks(m, trackOne)
	cmd := m.handleKey(tea.KeyPressMsg{Code: 'l', Text: "l"})
	if cmd == nil || m.flash != "" {
		t.Errorf("l: cmd = %v, flash = %q", cmd, m.flash)
	}
}
