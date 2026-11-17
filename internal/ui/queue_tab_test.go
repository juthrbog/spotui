package ui

import (
	"errors"
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"
	"github.com/charmbracelet/x/ansi"
	"github.com/devgianlu/go-librespot/daemon"
)

var trackThree = &daemon.ApiTrack{
	Uri: "spotify:track:3", Name: "Third Song", AlbumUri: "spotify:album:a3", AlbumName: "Album Three",
	ArtistUris: []string{"spotify:artist:w"}, ArtistNames: []string{"Band"},
}

func entry(provider daemon.QueueEntryProvider, uri string, t *daemon.ApiTrack) daemon.ApiQueueEntry {
	return daemon.ApiQueueEntry{Uri: uri, Provider: provider, Track: t}
}

// testQueue plays trackOne from a playlist, with trackTwo queued, then a
// local file and trackThree from the playlist, then an autoplay track.
func testQueue() *daemon.ApiQueue {
	now := entry(daemon.QueueEntryProviderContext, trackOne.Uri, trackOne)
	return &daemon.ApiQueue{
		Track: &now,
		NextTracks: []daemon.ApiQueueEntry{
			entry(daemon.QueueEntryProviderQueue, trackTwo.Uri, trackTwo),
			entry(daemon.QueueEntryProviderContext, "spotify:local:a:b:c:1", nil),
			entry(daemon.QueueEntryProviderContext, trackThree.Uri, trackThree),
			entry(daemon.QueueEntryProviderAutoplay, "spotify:track:9", &daemon.ApiTrack{Uri: "spotify:track:9", Name: "Radio Song"}),
		},
	}
}

// newQueueModel returns a model that plays trackOne from the playlist Mix,
// with the queue tab open and testQueue loaded.
func newQueueModel(t *testing.T) *Model {
	t.Helper()
	m := newGoToModel(t, trackOne)
	ctxURI, ctxName := "spotify:playlist:1", "Mix"
	m.Update(statusMsg{st: &daemon.ApiStatus{Username: "jakob", Track: trackOne, ContextUri: &ctxURI, ContextName: &ctxName}})
	pressKey(m, '3')
	m.Update(queueMsg{seq: m.queue.seq, q: testQueue()})
	return m
}

// selectSlot moves the cursor to the first row in slot whose URI is uri.
func selectSlot(t *testing.T, p *queuePage, slot queueSlot, uri string) {
	t.Helper()
	for i, r := range p.list.rows {
		if r.slot == slot && r.uri == uri {
			p.list.cursor = i
			return
		}
	}
	t.Fatalf("no row %s in slot %d", uri, slot)
}

func TestQueueTabShowsSections(t *testing.T) {
	m := newQueueModel(t)
	if m.tab != tabQueue || m.current() != m.queue {
		t.Fatalf("3 did not open the queue tab: tab = %d, page = %T", m.tab, m.current())
	}

	var headers []string
	slots := map[string]queueSlot{}
	for _, r := range m.queue.list.all {
		switch {
		case r.kind == rowHeader && len(r.cols) > 0:
			headers = append(headers, r.cols[0])
		case r.kind == rowItem:
			slots[r.uri] = r.slot
		}
	}
	want := []string{"Now playing", "Next in queue", "Next from Mix", "Next from autoplay"}
	if strings.Join(headers, "|") != strings.Join(want, "|") {
		t.Errorf("headers = %q, want %q", headers, want)
	}
	wantSlots := map[string]queueSlot{
		trackOne.Uri: slotNow, trackTwo.Uri: slotQueued, "spotify:local:a:b:c:1": slotContext,
		trackThree.Uri: slotContext, "spotify:track:9": slotAutoplay,
	}
	for uri, slot := range wantSlots {
		if slots[uri] != slot {
			t.Errorf("%s: slot %d, want %d", uri, slots[uri], slot)
		}
	}
	if got := m.queue.title(); got != "Queue · 1 queued track" {
		t.Errorf("title = %q", got)
	}
	if head := ansi.Strip(m.headerView(100)); !strings.Contains(head, "3 Queue") {
		t.Errorf("header has no queue tab: %q", head)
	}
}

func TestQueueTabWithoutQueuedTracks(t *testing.T) {
	m := newQueueModel(t)
	q := testQueue()
	q.NextTracks = q.NextTracks[2:]
	m.Update(queueMsg{seq: m.queue.seq, q: q})
	if !hasNote(m.queue.list.all, "No queued tracks") {
		t.Error("no note for an empty queue")
	}
	if got := m.queue.title(); got != "Queue" {
		t.Errorf("title = %q", got)
	}

	m.Update(queueMsg{seq: m.queue.seq, q: &daemon.ApiQueue{}})
	if !hasNote(m.queue.list.all, "Nothing plays") {
		t.Error("no note when nothing plays")
	}
}

func hasNote(rows []row, text string) bool {
	for _, r := range rows {
		if r.kind == rowNote && strings.Contains(strings.Join(r.cols, " "), text) {
			return true
		}
	}
	return false
}

func TestQueueTabDropsOldReplies(t *testing.T) {
	m := newQueueModel(t)
	old := m.queue.seq
	m.Update(engineEventMsg{ev: &daemon.ApiEvent{Type: daemon.ApiEventTypeQueue}})
	if m.queue.seq != old+1 {
		t.Fatalf("a queue event with the tab open did not load: seq %d, want %d", m.queue.seq, old+1)
	}
	m.Update(queueMsg{seq: old, q: &daemon.ApiQueue{}})
	if m.queue.q.Track == nil {
		t.Error("an old reply replaced the list")
	}
}

func TestQueueEventWithTabClosed(t *testing.T) {
	m := newQueueModel(t)
	pressKey(m, '1')
	seq := m.queue.seq
	m.Update(engineEventMsg{ev: &daemon.ApiEvent{Type: daemon.ApiEventTypeQueue}})
	if m.queue.seq != seq || !m.queue.stale {
		t.Fatalf("event with the tab closed: seq %d (was %d), stale %v", m.queue.seq, seq, m.queue.stale)
	}
	pressKey(m, '3')
	if m.queue.seq != seq+1 || m.queue.stale {
		t.Errorf("opening the tab after a change did not load: seq %d, stale %v", m.queue.seq, m.queue.stale)
	}
	// Without a change, the tab shows the last list.
	pressKey(m, '1')
	pressKey(m, '3')
	if m.queue.seq != seq+1 {
		t.Errorf("opening the tab without a change loaded again: seq %d", m.queue.seq)
	}
}

func TestQueueTabCycling(t *testing.T) {
	m := newGoToModel(t, nil)
	m.Update(tea.KeyPressMsg{Code: tea.KeyTab, Mod: tea.ModShift})
	if m.tab != tabQueue {
		t.Errorf("shift+tab from the library: tab %d, want the queue", m.tab)
	}
	m.Update(tea.KeyPressMsg{Code: tea.KeyTab})
	if m.tab != tabLibrary {
		t.Errorf("tab from the queue: tab %d, want the library", m.tab)
	}
}

func TestQueueTabEnter(t *testing.T) {
	m := newQueueModel(t)
	p := m.queue

	selectSlot(t, p, slotQueued, trackTwo.Uri)
	m.Update(tea.KeyPressMsg{Code: tea.KeyEnter})
	if !strings.Contains(m.flash, "queued track") {
		t.Errorf("enter on a queued track: flash = %q", m.flash)
	}

	selectSlot(t, p, slotAutoplay, "spotify:track:9")
	m.Update(tea.KeyPressMsg{Code: tea.KeyEnter})
	if !strings.Contains(m.flash, "autoplay track") {
		t.Errorf("enter on an autoplay track: flash = %q", m.flash)
	}

	m.flash = ""
	selectSlot(t, p, slotNow, trackOne.Uri)
	if cmd := p.playSelected(m); cmd != nil || m.flash != "" {
		t.Errorf("enter on the track that plays: cmd = %v, flash = %q", cmd, m.flash)
	}

	selectSlot(t, p, slotContext, trackThree.Uri)
	if cmd := p.playSelected(m); cmd == nil || m.flash != "" {
		t.Errorf("enter on a context track: cmd = %v, flash = %q", cmd, m.flash)
	}

	m.status.ContextUri = nil
	if p.playSelected(m); !strings.Contains(m.flash, "Nothing plays from") {
		t.Errorf("enter without a context: flash = %q", m.flash)
	}
}

func TestQueueTabActionsUseSelection(t *testing.T) {
	m := newQueueModel(t)
	selectSlot(t, m.queue, slotContext, "spotify:local:a:b:c:1")
	if item := m.actionItem(); item == nil || item.uri != "spotify:local:a:b:c:1" {
		t.Errorf("action item = %+v, want the local file", item)
	}
}

func TestQueueTabRendersAndFilters(t *testing.T) {
	m := newQueueModel(t)
	for _, size := range [][2]int{{120, 40}, {80, 24}, {40, 12}} {
		m.Update(tea.WindowSizeMsg{Width: size[0], Height: size[1]})
		frame := checkFrame(t, m, size[0], size[1], "queue")
		if size[0] >= 80 && !strings.Contains(frame, "Next in queue") {
			t.Errorf("%dx%d: no queue section in\n%s", size[0], size[1], frame)
		}
	}

	pressKey(m, 'f')
	for _, r := range "third" {
		pressKey(m, r)
	}
	if shown, _ := m.queue.list.counts(); shown != 1 {
		t.Errorf("filter shows %d rows, want 1", shown)
	}
}

func TestQueueTabRetriesAfterError(t *testing.T) {
	m := newQueueModel(t)
	m.Update(queueMsg{seq: m.queue.seq, err: errors.New("boom")})
	if m.queue.q == nil || !m.queue.stale {
		t.Fatalf("after a failed load: list kept %v, stale %v", m.queue.q != nil, m.queue.stale)
	}
	seq := m.queue.seq
	pressKey(m, '1')
	pressKey(m, '3')
	if m.queue.seq != seq+1 {
		t.Errorf("opening the tab after a failed load did not retry: seq %d", m.queue.seq)
	}
}
