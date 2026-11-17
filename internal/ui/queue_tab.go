package ui

import (
	"context"
	"strings"
	"time"

	tea "charm.land/bubbletea/v2"
	"github.com/devgianlu/go-librespot/daemon"

	"github.com/juthrbog/spotui/internal/engine"
)

// queueSlot is the section of the queue tab that a row is in.
type queueSlot int

const (
	slotNone     queueSlot = iota // not a row of the queue tab
	slotNow                       // the track that plays
	slotQueued                    // a track the user queued
	slotContext                   // a next track of the context
	slotAutoplay                  // a track that autoplay picked
)

type queueMsg struct {
	seq int // load that the reply belongs to
	q   *daemon.ApiQueue
	err error
}

func fetchQueue(ctx context.Context, eng *engine.Engine, seq int) tea.Cmd {
	return func() tea.Msg {
		ctx, cancel := context.WithTimeout(ctx, 5*time.Second)
		defer cancel()
		q, err := eng.Queue(ctx)
		return queueMsg{seq: seq, q: q, err: err}
	}
}

// queuePage shows the track that plays, the tracks the user queued, and
// the next tracks of the context. go-librespot sends a queue event when the
// list changes, also for a change from another Spotify app. The page loads
// the list again on that event while the tab is open, and otherwise when
// the tab opens next.
type queuePage struct {
	list   list
	filter filterBar

	q     *daemon.ApiQueue
	err   error
	seq   int  // counts the loads; replies of an older load are dropped
	stale bool // the list changed since the last load
}

func newQueuePage() *queuePage {
	return &queuePage{
		filter: newFilterBar(),
		stale:  true,
		list: list{columns: []column{
			{width: 1}, // ♥ for liked tracks
			{weight: 4},
			{weight: 3},
			{weight: 3},
			{width: 7, right: true},
		}},
	}
}

func (p *queuePage) title() string {
	if n := p.queued(); n > 0 {
		return "Queue · " + plural(n, "queued track")
	}
	return "Queue"
}

func (p *queuePage) typing() bool      { return p.filter.typing }
func (p *queuePage) stopTyping()       { p.filter.stop() }
func (p *queuePage) clearFilter() bool { return p.filter.clear(&p.list) }
func (p *queuePage) selection() *row   { return p.list.selected() }

// queued counts the tracks that the user queued.
func (p *queuePage) queued() int {
	if p.q == nil {
		return 0
	}
	n := 0
	for _, e := range p.q.NextTracks {
		if e.Provider == daemon.QueueEntryProviderQueue {
			n++
		}
	}
	return n
}

// load asks for the queue. Replies of older loads are dropped, so a slow
// reply never replaces a newer list.
func (p *queuePage) load(m *Model) tea.Cmd {
	if !m.ready {
		return nil
	}
	p.seq++
	p.stale = false
	return fetchQueue(m.ctx, m.eng, p.seq)
}

// changed marks the list as changed. It loads the list at once when the
// queue tab is open.
func (p *queuePage) changed(m *Model) tea.Cmd {
	p.stale = true
	if m.tab != tabQueue {
		return nil
	}
	return p.load(m)
}

func (p *queuePage) update(m *Model, msg tea.Msg) tea.Cmd {
	switch msg := msg.(type) {
	case queueMsg:
		if msg.seq != p.seq {
			return nil
		}
		p.err = msg.err
		if msg.err != nil {
			// Try again when the tab opens next.
			p.stale = true
			return nil
		}
		p.q = msg.q
		liked := m.wantLiked(p.uris())
		p.rebuild(m)
		return liked
	case likedStatesMsg, likedSetMsg:
		if p.q != nil {
			p.rebuild(m)
		}
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
			return p.playSelected(m)
		}
	default:
		if p.filter.typing {
			return p.filter.update(&p.list, msg)
		}
	}
	return nil
}

// playSelected skips to the selected track of the context. go-librespot
// keeps the queue when the context starts again. It cannot jump to a
// queued track or an autoplay track yet.
func (p *queuePage) playSelected(m *Model) tea.Cmd {
	r := p.list.selected()
	switch {
	case r == nil:
		return nil
	case r.slot == slotNow:
		return nil
	case r.slot == slotQueued:
		return m.setFlash("spotui cannot jump to a queued track yet. Press n to skip to the next track.", true)
	case r.slot == slotAutoplay:
		return m.setFlash("spotui cannot jump to an autoplay track yet. Press n to skip to the next track.", true)
	}
	contextURI := ""
	if m.status != nil {
		contextURI = deref(m.status.ContextUri)
	}
	if contextURI == "" {
		return m.setFlash("Nothing plays from a playlist, album, or artist.", true)
	}
	return m.play(contextURI, r.uri)
}

func (p *queuePage) uris() []string {
	var uris []string
	add := func(e *daemon.ApiQueueEntry) {
		if e != nil && isTrackURI(e.Uri) {
			uris = append(uris, e.Uri)
		}
	}
	add(p.q.Track)
	for i := range p.q.NextTracks {
		add(&p.q.NextTracks[i])
	}
	return uris
}

func (p *queuePage) rebuild(m *Model) {
	q := p.q
	var rows []row
	if q.Track == nil {
		rows = append(rows, note("Nothing plays. Pick a track and press enter."))
		p.list.setRows(rows)
		return
	}
	rows = append(rows, row{kind: rowHeader, cols: []string{"Now playing"}}, queueRow(m, q.Track, slotNow))

	var queued, context, autoplay []row
	for i := range q.NextTracks {
		e := &q.NextTracks[i]
		switch e.Provider {
		case daemon.QueueEntryProviderQueue:
			queued = append(queued, queueRow(m, e, slotQueued))
		case daemon.QueueEntryProviderAutoplay:
			autoplay = append(autoplay, queueRow(m, e, slotAutoplay))
		default:
			context = append(context, queueRow(m, e, slotContext))
		}
	}

	rows = append(rows, row{kind: rowHeader}, row{kind: rowHeader, cols: []string{"Next in queue"}})
	if len(queued) == 0 {
		rows = append(rows, note("No queued tracks. Press e on a track to add it."))
	}
	rows = append(rows, queued...)
	if len(context) > 0 {
		rows = append(rows, row{kind: rowHeader}, row{kind: rowHeader, cols: []string{"Next from " + m.contextName()}})
		rows = append(rows, context...)
	}
	if len(autoplay) > 0 {
		rows = append(rows, row{kind: rowHeader}, row{kind: rowHeader, cols: []string{"Next from autoplay"}})
		rows = append(rows, autoplay...)
	}
	p.list.setRows(rows)
}

// contextName names the playlist, album, or artist that plays.
func (m *Model) contextName() string {
	if m.status != nil {
		if name := deref(m.status.ContextName); name != "" {
			return name
		}
		if kind := contextKind(deref(m.status.ContextUri)); kind != "" {
			return "the " + strings.ToLower(kind)
		}
	}
	return "the context"
}

// queueRow returns the list row of a queue entry. The track details are
// missing until go-librespot has them, and for local files.
func queueRow(m *Model, e *daemon.ApiQueueEntry, slot queueSlot) row {
	heart := ""
	if m.liked[e.Uri] {
		heart = "♥"
	}
	t := e.Track
	if t == nil {
		return row{cols: []string{heart, "…"}, uri: e.Uri, slot: slot}
	}
	artists := strings.Join(t.ArtistNames, ", ")
	r := trackRow(t)
	r.cols = []string{heart, t.Name, artists, t.AlbumName, formatMs(int64(t.Duration))}
	r.uri = e.Uri
	r.slot = slot
	r.image = deref(t.AlbumCoverUrl)
	r.info = []string{t.Name, artists, t.AlbumName}
	return r
}

func (p *queuePage) view(m *Model, width, height int) string {
	head := ""
	if p.filter.active() {
		head = p.filter.view(&p.list, width) + "\n"
		height--
	}
	switch {
	case p.err != nil && p.q == nil:
		return head + styleError.Render("Could not load the queue: "+p.err.Error())
	case p.q == nil:
		return head + styleDim.Render("Loading the queue…")
	}
	return head + p.list.view(width, height, true, m.playingURI())
}
