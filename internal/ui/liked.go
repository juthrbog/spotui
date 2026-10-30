package ui

import (
	"context"
	"time"

	tea "charm.land/bubbletea/v2"
)

// likedBatch is the most URIs that one liked request takes.
const likedBatch = 50

type (
	likedStatesMsg struct {
		asked  []string
		states map[string]bool
		err    error
	}
	likedSetMsg struct {
		uri   string
		name  string
		liked bool
		err   error
	}
)

// wantLiked loads in the background whether the tracks in uris are liked.
// It skips URIs that are not tracks and URIs that it already asked for.
func (m *Model) wantLiked(uris []string) tea.Cmd {
	batches := m.likedBatches(uris)
	if len(batches) == 0 {
		return nil
	}
	cmds := make([]tea.Cmd, len(batches))
	for i, b := range batches {
		cmds[i] = m.loadLiked(b)
	}
	return tea.Batch(cmds...)
}

// likedBatches marks the new track URIs of uris as asked and splits them
// into batches for the player.
func (m *Model) likedBatches(uris []string) [][]string {
	var batches [][]string
	var batch []string
	for _, uri := range uris {
		if !isTrackURI(uri) || m.likedAsked[uri] {
			continue
		}
		m.likedAsked[uri] = true
		batch = append(batch, uri)
		if len(batch) == likedBatch {
			batches, batch = append(batches, batch), nil
		}
	}
	if len(batch) > 0 {
		batches = append(batches, batch)
	}
	return batches
}

func (m *Model) loadLiked(uris []string) tea.Cmd {
	ctx, eng := m.ctx, m.eng
	return func() tea.Msg {
		ctx, cancel := context.WithTimeout(ctx, 30*time.Second)
		defer cancel()
		states, err := eng.LikedStates(ctx, uris)
		return likedStatesMsg{asked: uris, states: states, err: err}
	}
}

// updateLiked keeps the liked states up to date. The pages get the same
// messages afterwards, to show the hearts.
func (m *Model) updateLiked(msg tea.Msg) tea.Cmd {
	switch msg := msg.(type) {
	case likedStatesMsg:
		if msg.err != nil {
			// The hearts are only a hint, so ask again the next time.
			for _, uri := range msg.asked {
				delete(m.likedAsked, uri)
			}
			return nil
		}
		for uri, liked := range msg.states {
			m.liked[uri] = liked
		}
	case likedSetMsg:
		delete(m.likePending, msg.uri)
		if msg.err != nil {
			return m.setFlash("Could not change Liked Songs: "+msg.err.Error(), true)
		}
		m.liked[msg.uri] = msg.liked
		if msg.liked {
			return m.setFlash("Added to Liked Songs: "+msg.name, false)
		}
		return m.setFlash("Removed from Liked Songs: "+msg.name, false)
	}
	return nil
}

func (m *Model) toggleLike() tea.Cmd { return m.toggleLikeOf(m.actionItem()) }

// toggleLikeOf likes the track of item, or unlikes it when it is liked. If
// the state is not known yet, it asks the player first.
func (m *Model) toggleLikeOf(item *row) tea.Cmd {
	switch {
	case item == nil:
		return m.setFlash("Select a track or play one first.", true)
	case !isTrackURI(item.uri):
		return m.setFlash("Only songs can be liked.", true)
	case !m.ready:
		return m.setFlash("The player is not ready yet.", true)
	case m.likePending[item.uri]:
		// The first change has not come back yet, so its state is not
		// known.
		return m.setFlash("Still changing Liked Songs for this track.", true)
	}
	uri, name := item.uri, trackName(item)
	m.likePending[uri] = true
	liked, known := m.liked[uri]
	ctx, eng := m.ctx, m.eng
	return func() tea.Msg {
		ctx, cancel := context.WithTimeout(ctx, 15*time.Second)
		defer cancel()
		if !known {
			states, err := eng.LikedStates(ctx, []string{uri})
			if err != nil {
				return likedSetMsg{uri: uri, name: name, err: err}
			}
			liked = states[uri]
		}
		err := eng.SetLiked(ctx, []string{uri}, !liked)
		return likedSetMsg{uri: uri, name: name, liked: !liked, err: err}
	}
}

// trackName names the track of item in a message. A row that is still
// loading has no name yet.
func trackName(item *row) string {
	if item.label == "" {
		return "the track"
	}
	return item.label
}

// likeLabel names the like action for the track with the URI uri.
func likeLabel(liked map[string]bool, uri string) string {
	switch l, known := liked[uri]; {
	case !known:
		return "Like or unlike"
	case l:
		return "Unlike"
	}
	return "Like"
}
