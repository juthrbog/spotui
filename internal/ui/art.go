package ui

import (
	"bytes"
	"context"
	"image"
	"net/http"
	"slices"
	"strings"
	"time"

	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
	uv "github.com/charmbracelet/ultraviolet"
	"github.com/charmbracelet/x/ansi"

	"github.com/juthrbog/spotui/internal/art"
)

const (
	// Kitty image ids are sent as 256-color foreground values. Ids below 16
	// are avoided because some renderers rewrite them as basic colors.
	firstKittyID = 16
	lastKittyID  = 255

	maxCachedImages = 64
	maxCachedBlocks = 64

	thumbCols, thumbRows = 6, 3

	detectTimeout = 1500 * time.Millisecond
)

type artKey struct {
	url        string
	cols, rows int
}

type (
	artLoadedMsg struct {
		url string
		img image.Image
		err error
	}
	artDetectTimeoutMsg struct{}
)

// artState keeps downloaded images and what the terminal already has. Only
// Update changes it; commands get copies of what they need.
type artState struct {
	setting   string
	mode      art.Mode
	detecting bool
	client    *http.Client

	images     map[string]image.Image
	imageOrder []string // oldest first
	loading    map[string]bool
	failed     map[string]bool

	blocks     map[artKey]string
	blockOrder []artKey

	ids     map[artKey]int
	idOrder []artKey // least recently used first
}

func newArtState(setting string) *artState {
	a := &artState{
		setting: setting,
		client:  &http.Client{Timeout: 15 * time.Second},
		images:  map[string]image.Image{},
		loading: map[string]bool{},
		failed:  map[string]bool{},
		blocks:  map[artKey]string{},
		ids:     map[artKey]int{},
	}
	switch setting {
	case "kitty":
		a.mode = art.ModeKitty
	case "blocks":
		a.mode = art.ModeBlocks
	case "off":
		a.mode = art.ModeOff
	default:
		a.detecting = true
	}
	return a
}

// enabled reports whether cover art is on or still being detected.
func (a *artState) enabled() bool { return a.detecting || a.mode != art.ModeOff }

// init asks the terminal whether it supports kitty graphics.
func (a *artState) init() tea.Cmd {
	if !a.detecting {
		return nil
	}
	return tea.Batch(
		tea.Raw(art.KittyQuery()),
		tea.Tick(detectTimeout, func(time.Time) tea.Msg { return artDetectTimeoutMsg{} }),
	)
}

// update handles art messages and reports whether msg was one.
func (a *artState) update(msg tea.Msg) bool {
	switch msg := msg.(type) {
	case uv.KittyGraphicsEvent:
		if msg.Options.ID != art.QueryID {
			return false
		}
		if a.detecting {
			a.detecting = false
			a.mode = art.ModeBlocks
			if bytes.HasPrefix(msg.Payload, []byte("OK")) && !art.PlaceholdersUnsupported() {
				a.mode = art.ModeKitty
			}
		}
		return true
	case artDetectTimeoutMsg:
		if a.detecting {
			a.detecting = false
			a.mode = art.ModeBlocks
		}
		return true
	case artLoadedMsg:
		delete(a.loading, msg.url)
		if msg.err != nil {
			a.failed[msg.url] = true
			return true
		}
		a.images[msg.url] = msg.img
		a.imageOrder = append(a.imageOrder, msg.url)
		if len(a.imageOrder) > maxCachedImages {
			delete(a.images, a.imageOrder[0])
			a.imageOrder = a.imageOrder[1:]
		}
		return true
	}
	return false
}

// sync downloads, renders, or sends to the terminal the images in wants.
func (a *artState) sync(ctx context.Context, wants []artKey) tea.Cmd {
	if a.detecting || a.mode == art.ModeOff {
		return nil
	}
	var cmds []tea.Cmd
	for _, k := range wants {
		if k.url == "" || k.cols <= 0 || k.rows <= 0 {
			continue
		}
		switch a.mode {
		case art.ModeKitty:
			if _, ok := a.ids[k]; ok {
				a.touchID(k)
				continue
			}
		case art.ModeBlocks:
			if _, ok := a.blocks[k]; ok {
				continue
			}
		}

		img, ok := a.images[k.url]
		if !ok {
			if !a.loading[k.url] && !a.failed[k.url] {
				a.loading[k.url] = true
				cmds = append(cmds, fetchArt(ctx, a.client, k.url))
			}
			continue
		}

		switch a.mode {
		case art.ModeKitty:
			id, evicted := a.allocID(k)
			cmds = append(cmds, transmitArt(img, id, k, evicted))
		case art.ModeBlocks:
			a.blocks[k] = art.Blocks(img, k.cols, k.rows)
			a.blockOrder = append(a.blockOrder, k)
			if len(a.blockOrder) > maxCachedBlocks {
				delete(a.blocks, a.blockOrder[0])
				a.blockOrder = a.blockOrder[1:]
			}
		}
	}
	return tea.Batch(cmds...)
}

// allocID gives k a kitty image id. When all ids are in use, it takes the id
// of the image that was shown longest ago.
func (a *artState) allocID(k artKey) (id int, evicted bool) {
	if len(a.ids) <= lastKittyID-firstKittyID {
		id = firstKittyID + len(a.ids)
	} else {
		old := a.idOrder[0]
		a.idOrder = a.idOrder[1:]
		id, evicted = a.ids[old], true
		delete(a.ids, old)
	}
	a.ids[k] = id
	a.idOrder = append(a.idOrder, k)
	return id, evicted
}

func (a *artState) touchID(k artKey) {
	if i := slices.Index(a.idOrder, k); i >= 0 && i < len(a.idOrder)-1 {
		a.idOrder = append(slices.Delete(a.idOrder, i, i+1), k)
	}
}

// view returns the text that draws k, if it is ready.
func (a *artState) view(k artKey) (string, bool) {
	switch a.mode {
	case art.ModeKitty:
		if id, ok := a.ids[k]; ok {
			return art.KittyPlaceholder(id, k.cols, k.rows), true
		}
	case art.ModeBlocks:
		s, ok := a.blocks[k]
		return s, ok
	}
	return "", false
}

// cleanup returns the sequence that frees every image spotui sent.
func (a *artState) cleanup() string {
	var b strings.Builder
	for _, id := range a.ids {
		b.WriteString(art.KittyDelete(id))
	}
	return b.String()
}

func fetchArt(ctx context.Context, client *http.Client, url string) tea.Cmd {
	return func() tea.Msg {
		img, err := art.Fetch(ctx, client, url)
		return artLoadedMsg{url: url, img: img, err: err}
	}
}

// transmitArt encodes the image off the UI loop and sends it to the
// terminal.
func transmitArt(img image.Image, id int, k artKey, replace bool) tea.Cmd {
	return func() tea.Msg {
		seq, err := art.KittyTransmit(img, id, k.cols, k.rows)
		if err != nil {
			return nil
		}
		if replace {
			seq = art.KittyDelete(id) + seq
		}
		return tea.RawMsg{Msg: seq}
	}
}

// emptyArt fills a box with a centered symbol while there is no image.
func emptyArt(cols, rows int, glyph string) string {
	return lipgloss.Place(cols, rows, lipgloss.Center, lipgloss.Center, styleDim.Render(glyph))
}

// artSubject is what the side panel shows.
type artSubject struct {
	image   string
	glyph   string
	heading string
	info    []string
}

// panelLayout returns the width of the side panel and the size of its art
// box. ok is false when the terminal is too small or cover art is off.
func (m *Model) panelLayout() (width, cols, rows int, ok bool) {
	if !m.art.enabled() || m.width < 90 {
		return 0, 0, 0, false
	}
	cols = min(max(m.width*3/10, 20), 44) &^ 1
	rows = cols / 2
	// Keep five lines under the art for the text.
	if maxRows := m.height - chromeHeight - 5; rows > maxRows {
		rows = maxRows
		cols = rows * 2
	}
	if rows < 5 {
		return 0, 0, 0, false
	}
	return cols, cols, rows, true
}

// panelSubject is the selected item, or the playing track if nothing is
// selected.
func (m *Model) panelSubject() artSubject {
	if sel := m.current().selection(); sel != nil {
		glyph := sel.glyph
		if glyph == "" {
			glyph = "♪"
		}
		return artSubject{image: sel.image, glyph: glyph, info: sel.info}
	}
	s := artSubject{glyph: "♪", heading: "Now playing"}
	if st := m.status; st != nil && st.Track != nil {
		s.image = deref(st.Track.AlbumCoverUrl)
		s.info = []string{st.Track.Name, strings.Join(st.Track.ArtistNames, ", "), st.Track.AlbumName}
	} else {
		s.info = []string{"Nothing playing"}
	}
	return s
}

func (m *Model) showPanel() bool {
	_, _, _, ok := m.panelLayout()
	return ok && m.ready && !m.help && m.auth == nil && m.webLoginURL == ""
}

func (m *Model) panelView(width int) string {
	_, cols, rows, _ := m.panelLayout()
	s := m.panelSubject()
	box, ok := m.art.view(artKey{url: s.image, cols: cols, rows: rows})
	if !ok {
		box = emptyArt(cols, rows, s.glyph)
	}
	lines := []string{box, ""}
	if s.heading != "" {
		lines = append(lines, styleAccent.Render(s.heading))
	}
	for i, l := range s.info {
		if l == "" {
			continue
		}
		l = ansi.Truncate(l, width, "…")
		if i == 0 {
			lines = append(lines, styleBold.Render(l))
		} else {
			lines = append(lines, styleDim.Render(l))
		}
	}
	return strings.Join(lines, "\n")
}

// artWants lists the images the next frame shows.
func (m *Model) artWants() []artKey {
	var wants []artKey
	if m.showPanel() {
		_, cols, rows, _ := m.panelLayout()
		if url := m.panelSubject().image; url != "" {
			wants = append(wants, artKey{url: url, cols: cols, rows: rows})
		}
	}
	if url := m.thumbURL(); url != "" {
		wants = append(wants, artKey{url: url, cols: thumbCols, rows: thumbRows})
	}
	return wants
}

// thumbURL is the cover of the playing track for the now-playing bar. Only
// kitty graphics are sharp enough at this size.
func (m *Model) thumbURL() string {
	if m.art.mode != art.ModeKitty || m.status == nil || m.status.Track == nil {
		return ""
	}
	return deref(m.status.Track.AlbumCoverUrl)
}

func deref(s *string) string {
	if s == nil {
		return ""
	}
	return *s
}

// joinColumns puts two blocks of equal height side by side.
func joinColumns(left string, leftWidth int, sep, right string) string {
	l, r := strings.Split(left, "\n"), strings.Split(right, "\n")
	out := make([]string, len(l))
	for i := range l {
		out[i] = fit(l[i], leftWidth, false) + sep
		if i < len(r) {
			out[i] += r[i]
		}
	}
	return strings.Join(out, "\n")
}
