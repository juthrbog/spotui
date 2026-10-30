package ui

import (
	"context"
	"errors"
	"strings"
	"time"

	tea "charm.land/bubbletea/v2"
	"github.com/devgianlu/go-librespot/daemon"
)

type playlistAddMsg struct {
	playlistURI string
	playlist    string
	track       string
	err         error
}

func isEpisodeURI(uri string) bool { return strings.HasPrefix(uri, "spotify:episode:") }

// canAddToPlaylist reports whether item can go into a playlist.
func canAddToPlaylist(item *row) bool { return isTrackURI(item.uri) || isEpisodeURI(item.uri) }

func (m *Model) openPlaylistPicker(item *row) tea.Cmd {
	if item == nil || !canAddToPlaylist(item) {
		return m.setFlash("Only songs and episodes can go into a playlist.", true)
	}
	return m.push(newPlaylistPicker(m, *item))
}

// addToPlaylist adds item to the end of the playlist with the URI uri.
func (m *Model) addToPlaylist(item row, uri, name string) tea.Cmd {
	if !m.ready {
		return m.setFlash("The player is not ready yet.", true)
	}
	ctx, eng, track := m.ctx, m.eng, trackName(&item)
	return func() tea.Msg {
		ctx, cancel := context.WithTimeout(ctx, 30*time.Second)
		defer cancel()
		err := eng.PlaylistAddTracks(ctx, uri, []string{item.uri})
		return playlistAddMsg{playlistURI: uri, playlist: name, track: track, err: err}
	}
}

// updatePlaylists shows the result of a change to a playlist. The pages get
// the same message afterwards, so an open page of the playlist loads again.
func (m *Model) updatePlaylists(msg tea.Msg) tea.Cmd {
	add, ok := msg.(playlistAddMsg)
	switch {
	case !ok:
		return nil
	case errors.Is(add.err, daemon.ErrForbidden):
		return m.setFlash("You cannot change the playlist "+add.playlist+".", true)
	case add.err != nil:
		return m.setFlash("Could not add to "+add.playlist+": "+add.err.Error(), true)
	}
	return m.setFlash("Added "+add.track+" to "+add.playlist+".", false)
}

// playlistPicker asks which playlist to add a track to. It lists only the
// playlists that the user can change.
type playlistPicker struct {
	item   row
	list   list
	filter filterBar
}

func newPlaylistPicker(m *Model, item row) *playlistPicker {
	p := &playlistPicker{
		item:   item,
		list:   list{columns: []column{{weight: 3}, {weight: 2}, {width: 11, right: true}}},
		filter: newFilterBar(),
	}
	p.rebuild(m)
	return p
}

func (p *playlistPicker) title() string     { return "Add " + trackName(&p.item) + " to" }
func (p *playlistPicker) typing() bool      { return p.filter.typing }
func (p *playlistPicker) stopTyping()       { p.filter.stop() }
func (p *playlistPicker) clearFilter() bool { return p.filter.clear(&p.list) }
func (p *playlistPicker) selection() *row   { return p.list.selected() }

func (p *playlistPicker) update(m *Model, msg tea.Msg) tea.Cmd {
	switch msg := msg.(type) {
	case playlistsMsg:
		p.rebuild(m)
	case tea.KeyPressMsg:
		if p.filter.typing {
			return p.filter.update(&p.list, msg)
		}
		if p.list.handleKey(msg.String()) {
			return nil
		}
		switch msg.String() {
		case "f":
			return p.filter.start()
		case "enter":
			if r := p.list.selected(); r != nil {
				m.pop()
				return m.addToPlaylist(p.item, r.uri, r.label)
			}
		}
	default:
		if p.filter.typing {
			return p.filter.update(&p.list, msg)
		}
	}
	return nil
}

func (p *playlistPicker) rebuild(m *Model) {
	lib := m.library
	var rows []row
	for _, pl := range lib.playlists {
		if pl.CanEdit {
			rows = append(rows, playlistRow(pl, m.username()))
		}
	}
	switch {
	case len(rows) > 0:
	case lib.playlistsLoading || !m.ready:
		rows = append(rows, note("Loading playlists…"))
	case lib.playlistsErr != nil:
		rows = append(rows, note("Could not load playlists: "+lib.playlistsErr.Error()+" (press R to retry)"))
	default:
		rows = append(rows, note("No playlists that you can change."))
	}
	p.list.setRows(rows)
}

func (p *playlistPicker) view(m *Model, width, height int) string {
	if !p.filter.active() {
		return p.list.view(width, height, true, "")
	}
	return p.filter.view(&p.list, width) + "\n" + p.list.view(width, height-1, true, "")
}
