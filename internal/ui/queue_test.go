package ui

import (
	"errors"
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"
	"github.com/charmbracelet/x/ansi"
	"github.com/devgianlu/go-librespot/daemon"
)

func TestAddToQueueChecksItem(t *testing.T) {
	m := newGoToModel(t, nil)
	if m.addToQueueOf(nil); m.flash == "" {
		t.Error("no message without an item")
	}
	m.flash = ""
	if m.addToQueueOf(&row{uri: "spotify:album:a"}); !strings.Contains(m.flash, "Only songs and episodes") {
		t.Errorf("album: flash = %q", m.flash)
	}
	m.flash = ""
	m.ready = false
	if m.addToQueueOf(&row{uri: trackOne.Uri}); !strings.Contains(m.flash, "not ready") {
		t.Errorf("not ready: flash = %q", m.flash)
	}
	m.ready, m.flash = true, ""
	if cmd := m.addToQueueOf(&row{uri: "spotify:episode:e"}); cmd == nil || m.flash != "" {
		t.Errorf("episode: cmd = %v, flash = %q", cmd, m.flash)
	}
}

func TestQueueKeyInTrackList(t *testing.T) {
	m := newGoToModel(t, nil)
	openTracks(m, trackOne)
	if cmd := m.handleKey(tea.KeyPressMsg{Code: 'e', Text: "e"}); cmd == nil || m.flash != "" {
		t.Errorf("e: cmd = %v, flash = %q", cmd, m.flash)
	}
}

func TestQueueAddedMessages(t *testing.T) {
	m := newGoToModel(t, nil)
	cases := []struct {
		msg   queueAddMsg
		flash string
		isErr bool
	}{
		{queueAddMsg{track: "Song"}, "Added to the queue: Song", false},
		{queueAddMsg{track: "Song", played: true}, "Nothing was playing, so Song plays now.", false},
		{queueAddMsg{track: "Song", err: errors.New("boom")}, "Could not add to the queue: boom", true},
	}
	for _, c := range cases {
		if cmd := m.update(c.msg); cmd == nil {
			t.Errorf("%+v: no status refresh", c.msg)
		}
		if m.flash != c.flash || m.flashErr != c.isErr {
			t.Errorf("%+v: flash = %q (error %v), want %q", c.msg, m.flash, m.flashErr, c.flash)
		}
	}
}

func TestNowPlayingShowsNextTrack(t *testing.T) {
	m := newGoToModel(t, nil)
	m.Update(statusMsg{st: &daemon.ApiStatus{Username: "jakob", Track: trackOne, NextTrack: trackTwo}})
	frame := ansi.Strip(checkFrame(t, m, 100, 30, "next track"))
	if !strings.Contains(frame, "next: Second Song") {
		t.Errorf("frame does not show the next track:\n%s", frame)
	}
	m.Update(tea.WindowSizeMsg{Width: 40, Height: 12})
	checkFrame(t, m, 40, 12, "next track small")
}

// TestLongNextTrackKeepsDetails makes sure that a long name of the next
// track does not push the details of the playing track off the line.
func TestLongNextTrackKeepsDetails(t *testing.T) {
	m := newGoToModel(t, nil)
	long := &daemon.ApiTrack{Uri: "spotify:track:9", Name: strings.Repeat("Very Long Episode Title ", 6)}
	m.Update(statusMsg{st: &daemon.ApiStatus{Username: "jakob", Track: trackOne, NextTrack: long}})
	for _, w := range []int{80, 120} {
		lines := strings.Split(ansi.Strip(m.nowPlayingLines(w)), "\n")
		if !strings.Contains(lines[1], "Solo Artist · Album One") || !strings.Contains(lines[1], "next: Very") {
			t.Errorf("width %d: line 2 = %q", w, lines[1])
		}
		if ansi.StringWidth(lines[1]) > w {
			t.Errorf("width %d: line 2 is %d cells", w, ansi.StringWidth(lines[1]))
		}
	}
	if lines := strings.Split(ansi.Strip(m.nowPlayingLines(30)), "\n"); strings.Contains(lines[1], "next:") {
		t.Errorf("narrow line 2 still has the hint: %q", lines[1])
	}
}

func TestContextLoaded(t *testing.T) {
	uri := "spotify:playlist:1"
	cases := []struct {
		name string
		st   daemon.ApiStatus
		want bool
	}{
		{"nothing", daemon.ApiStatus{}, false},
		{"track", daemon.ApiStatus{Track: trackOne}, true},
		{"next track loading", daemon.ApiStatus{ContextUri: &uri}, true},
	}
	for _, c := range cases {
		if got := contextLoaded(&c.st); got != c.want {
			t.Errorf("%s: %v, want %v", c.name, got, c.want)
		}
	}
}
