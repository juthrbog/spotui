package ui

import (
	"context"
	"errors"
	"strings"
	"time"

	tea "charm.land/bubbletea/v2"
	"github.com/devgianlu/go-librespot/daemon"
	"github.com/devgianlu/go-librespot/spclient"
)

// playlistChangedMsg is the result of an add to or a remove from a
// playlist.
type playlistChangedMsg struct {
	playlistURI string
	playlist    string
	track       string
	removed     bool
	err         error
}

// conflict reports whether the playlist changed before a remove, so that
// the item was no longer at its position.
func (msg playlistChangedMsg) conflict() bool {
	return errors.Is(msg.err, spclient.ErrPlaylistConflict)
}

// countChange is how the number of items in the playlist changed.
func (msg playlistChangedMsg) countChange() int32 {
	switch {
	case msg.err != nil:
		return 0
	case msg.removed:
		return -1
	}
	return 1
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
		return playlistChangedMsg{playlistURI: uri, playlist: name, track: track, err: err}
	}
}

// editablePlaylist returns the playlist of the track list on screen, if the
// user can change it.
func (m *Model) editablePlaylist() ref {
	p, ok := m.current().(*tracksPage)
	if !ok || !strings.HasPrefix(p.uri, "spotify:playlist:") {
		return ref{}
	}
	for _, pl := range m.library.playlists {
		if pl.Uri == p.uri && pl.CanEdit {
			return ref{uri: pl.Uri, name: pl.Name}
		}
	}
	return ref{}
}

// confirmRemove asks before it removes item from playlist. Adding the item
// again changes its position and date, so the remove is not easy to undo.
func (m *Model) confirmRemove(item *row, playlist ref) tea.Cmd {
	it := *item
	position := m.removePosition(&it)
	m.confirm = &confirmPrompt{
		question: "Remove " + trackName(&it) + " from " + playlist.name + "?",
		yes:      func() tea.Cmd { return m.removeFromPlaylist(it, playlist, position) },
	}
	return nil
}

// removePosition returns the position to send with a remove of item, or
// nil to remove the item by its URI. go-librespot checks the position
// against the full playlist, but its track list leaves out local files, so
// a position is only sent when the list has the item more than one time.
func (m *Model) removePosition(item *row) *int {
	p, ok := m.current().(*tracksPage)
	if !ok || p.resp == nil {
		return nil
	}
	copies := 0
	for _, it := range p.resp.Tracks {
		if it.Uri == item.uri {
			copies++
		}
	}
	if copies < 2 {
		return nil
	}
	pos := item.pos
	return &pos
}

// removeFromPlaylist removes item from playlist: the copy at position, or
// the first copy when position is nil.
func (m *Model) removeFromPlaylist(item row, playlist ref, position *int) tea.Cmd {
	if !m.ready {
		return m.setFlash("The player is not ready yet.", true)
	}
	ctx, eng, track := m.ctx, m.eng, trackName(&item)
	return func() tea.Msg {
		ctx, cancel := context.WithTimeout(ctx, 30*time.Second)
		defer cancel()
		err := eng.PlaylistRemoveTrack(ctx, playlist.uri, item.uri, position)
		return playlistChangedMsg{playlistURI: playlist.uri, playlist: playlist.name, track: track, removed: true, err: err}
	}
}

// updatePlaylists shows the result of a change to a playlist. The pages get
// the same message afterwards, so an open page of the playlist loads again.
func (m *Model) updatePlaylists(msg tea.Msg) tea.Cmd {
	ch, ok := msg.(playlistChangedMsg)
	switch {
	case !ok:
		return nil
	case errors.Is(ch.err, daemon.ErrForbidden):
		return m.setFlash("You cannot change the playlist "+ch.playlist+".", true)
	case ch.conflict():
		// The playlist changed in another app, or it has a local file
		// before the item. go-librespot keeps its old copy of the list
		// after a failed remove, so a reload would not help.
		return m.setFlash("The selected copy of "+ch.track+" is not at its place in "+ch.playlist+", so nothing was removed. Remove it in the Spotify app.", true)
	case errors.Is(ch.err, daemon.ErrNotFound) && ch.removed:
		return m.setFlash(ch.track+" is no longer in "+ch.playlist+".", true)
	case ch.err != nil && ch.removed:
		return m.setFlash("Could not remove from "+ch.playlist+": "+ch.err.Error(), true)
	case ch.err != nil:
		return m.setFlash("Could not add to "+ch.playlist+": "+ch.err.Error(), true)
	case ch.removed:
		return m.setFlash("Removed "+ch.track+" from "+ch.playlist+".", false)
	}
	return m.setFlash("Added "+ch.track+" to "+ch.playlist+".", false)
}

// confirmPrompt is a yes or no question in the footer.
type confirmPrompt struct {
	question string
	yes      func() tea.Cmd
}

// confirmKey handles a key while a question is open. Other keys do nothing,
// so the question cannot be answered by mistake.
func (m *Model) confirmKey(key string) tea.Cmd {
	c := m.confirm
	switch key {
	case "y", "Y":
		m.confirm = nil
		return c.yes()
	case "n", "N", "esc":
		m.confirm = nil
	}
	return nil
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
