package ui

import (
	"context"
	"time"

	tea "charm.land/bubbletea/v2"
	"github.com/devgianlu/go-librespot/daemon"
)

type queueAddMsg struct {
	track  string
	played bool // nothing was loaded, so the item plays now
	err    error
}

func (m *Model) addToQueue() tea.Cmd { return m.addToQueueOf(m.actionItem()) }

// addToQueueOf adds item to the end of the queue. When nothing is loaded,
// go-librespot drops a queue request without an error, so the item plays
// at once instead.
func (m *Model) addToQueueOf(item *row) tea.Cmd {
	switch {
	case item == nil:
		return m.setFlash("Select a track or play one first.", true)
	case !isSongOrEpisode(item):
		return m.setFlash("Only songs and episodes can go into the queue.", true)
	case !m.ready:
		return m.setFlash("The player is not ready yet.", true)
	}
	uri, name := item.uri, trackName(item)
	ctx, eng := m.ctx, m.eng
	return func() tea.Msg {
		ctx, cancel := context.WithTimeout(ctx, 15*time.Second)
		defer cancel()
		st, err := eng.Status(ctx)
		if err != nil {
			return queueAddMsg{track: name, err: err}
		}
		if !contextLoaded(st) {
			return queueAddMsg{track: name, played: true, err: eng.Play(ctx, uri, "")}
		}
		return queueAddMsg{track: name, err: eng.AddToQueue(ctx, uri)}
	}
}

// contextLoaded reports whether the player has a context, so that it takes
// a queue add. Track alone is not enough: it is empty while the next track
// loads, for example after a skip, but the context and the queue stay.
func contextLoaded(st *daemon.ApiStatus) bool {
	return st.Track != nil || deref(st.ContextUri) != ""
}

func (m *Model) queueAdded(msg queueAddMsg) tea.Cmd {
	var flash tea.Cmd
	switch {
	case msg.err != nil:
		flash = m.setFlash("Could not add to the queue: "+msg.err.Error(), true)
	case msg.played:
		flash = m.setFlash("Nothing was playing, so "+msg.track+" plays now.", false)
	default:
		flash = m.setFlash("Added to the queue: "+msg.track, false)
	}
	// The status shows the next track.
	return tea.Batch(flash, fetchStatus(m.ctx, m.eng, false))
}
