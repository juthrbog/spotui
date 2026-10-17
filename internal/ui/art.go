package ui

import (
	"bytes"
	"context"
	"fmt"
	"image"
	"net/http"
	"runtime"
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
	maxCachedSixels = 16

	thumbCols, thumbRows = 6, 3

	detectTimeout = 1500 * time.Millisecond

	// sixelDelay is the time from the frame that empties a box to the sixel
	// image in it. Bubble Tea writes a frame about every 16 ms, and it
	// writes raw output before the frame of the same tick. The delay makes
	// sure that the image comes after the empty box and is not written over.
	sixelDelay = 100 * time.Millisecond
)

// sixelAuto is true where "auto" may pick sixel. Sixel is tested only in
// Windows Terminal. Elsewhere, cover_art = "sixel" turns it on.
var sixelAuto = runtime.GOOS == "windows"

// defaultCell is the cell size of the VT340, in pixels. Windows Terminal
// reports it too. It is used until the terminal reports its own size.
var defaultCell = image.Pt(10, 20)

type artKey struct {
	url        string
	cols, rows int
}

// artWant is an image the next frame shows, with the screen cell of its
// top-left corner. Only sixel mode uses the position.
type artWant struct {
	artKey
	x, y int
}

// sixelSlot is the sixel image a frame shows.
type sixelSlot struct {
	key  artKey
	x, y int
	img  art.SixelImage
}

type (
	artLoadedMsg struct {
		url string
		img image.Image
		err error
	}
	artDetectTimeoutMsg struct{}

	sixelEncodedMsg struct {
		key  artKey
		cell image.Point
		img  art.SixelImage
		err  error
	}
	sixelDrawMsg struct{ gen int }
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

	// Sixel mode shows one image: the cover in the side panel. The frame
	// fills its box with marked spaces (see sixelBox), and the image goes
	// out sixelDelay later. The renderer leaves the marked spaces alone, so
	// the image stays until the frame changes the box.
	cell       image.Point // zero until the terminal reports it
	sixels     map[artKey]art.SixelImage
	sixelOrder []artKey
	encoding   map[artKey]bool
	slot       *sixelSlot // the image of the frame, drawn or scheduled
	drawGen    int        // only the latest scheduled draw goes out
	parity     bool       // flipped to make the renderer erase the box
}

func newArtState(setting string) *artState {
	a := &artState{
		setting:  setting,
		client:   &http.Client{Timeout: 15 * time.Second},
		images:   map[string]image.Image{},
		loading:  map[string]bool{},
		failed:   map[string]bool{},
		blocks:   map[artKey]string{},
		ids:      map[artKey]int{},
		sixels:   map[artKey]art.SixelImage{},
		encoding: map[artKey]bool{},
	}
	switch setting {
	case "kitty":
		a.mode = art.ModeKitty
	case "sixel":
		a.mode = art.ModeSixel
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

// init asks the terminal what images it supports, and the size of its
// cells for sixel.
func (a *artState) init() tea.Cmd {
	if a.mode == art.ModeSixel {
		return tea.Raw(art.CellSizeQuery())
	}
	if !a.detecting {
		return nil
	}
	// The terminal answers in order, and every terminal answers DA1. So
	// when the DA1 answer comes, the kitty answer came first or never.
	return tea.Batch(
		tea.Raw(art.KittyQuery()+art.CellSizeQuery()+ansi.RequestPrimaryDeviceAttributes),
		tea.Tick(detectTimeout, func(time.Time) tea.Msg { return artDetectTimeoutMsg{} }),
	)
}

// update handles art messages. It reports whether msg was one, with the
// command that follows from it.
func (a *artState) update(msg tea.Msg) (bool, tea.Cmd) {
	switch msg := msg.(type) {
	case uv.KittyGraphicsEvent:
		if msg.Options.ID != art.QueryID {
			return false, nil
		}
		if a.detecting && bytes.HasPrefix(msg.Payload, []byte("OK")) && !art.PlaceholdersUnsupported() {
			a.detecting = false
			a.mode = art.ModeKitty
		}
		return true, nil
	case uv.PrimaryDeviceAttributesEvent:
		if a.detecting {
			a.detecting = false
			a.mode = art.ModeBlocks
			if sixelAuto && art.SixelSupported(msg) {
				a.mode = art.ModeSixel
			}
		}
		return true, nil
	case uv.CellSizeEvent:
		if c := image.Pt(msg.Width, msg.Height); c.X > 0 && c.Y > 0 && c != a.cell {
			a.cell = c
			a.resetSixels()
		}
		return true, nil
	case artDetectTimeoutMsg:
		if a.detecting {
			a.detecting = false
			a.mode = art.ModeBlocks
		}
		return true, nil
	case artLoadedMsg:
		delete(a.loading, msg.url)
		if msg.err != nil {
			a.failed[msg.url] = true
			return true, nil
		}
		a.images[msg.url] = msg.img
		a.imageOrder = append(a.imageOrder, msg.url)
		if len(a.imageOrder) > maxCachedImages {
			delete(a.images, a.imageOrder[0])
			a.imageOrder = a.imageOrder[1:]
		}
		return true, nil
	case sixelEncodedMsg:
		if msg.cell != a.cellSize() {
			return true, nil // made for an old cell size
		}
		if msg.err != nil {
			// The key stays in encoding, so it is not tried again.
			return true, nil
		}
		delete(a.encoding, msg.key)
		a.sixels[msg.key] = msg.img
		a.sixelOrder = append(a.sixelOrder, msg.key)
		if len(a.sixelOrder) > maxCachedSixels {
			delete(a.sixels, a.sixelOrder[0])
			a.sixelOrder = a.sixelOrder[1:]
		}
		return true, nil
	case sixelDrawMsg:
		if msg.gen != a.drawGen || a.slot == nil {
			return true, nil
		}
		return true, tea.Raw(a.slot.img.DrawAt(a.slot.x, a.slot.y))
	}
	return false, nil
}

// resize handles a new terminal size. The renderer then draws the whole
// screen again, which erases sixel images.
func (a *artState) resize() tea.Cmd {
	if a.mode != art.ModeSixel {
		return nil
	}
	a.slot = nil
	a.drawGen++
	// Some terminals change the cell size together with the font size.
	return tea.Raw(art.CellSizeQuery())
}

func (a *artState) cellSize() image.Point {
	if a.cell == (image.Point{}) {
		return defaultCell
	}
	return a.cell
}

// resetSixels forgets the sixel images, which have the old cell size.
func (a *artState) resetSixels() {
	a.sixels = map[artKey]art.SixelImage{}
	a.sixelOrder = nil
	a.encoding = map[artKey]bool{}
	a.slot = nil
	a.drawGen++
}

// sync downloads, renders, or sends to the terminal the images in wants.
func (a *artState) sync(ctx context.Context, wants []artWant) tea.Cmd {
	if a.detecting || a.mode == art.ModeOff {
		return nil
	}
	var cmds []tea.Cmd
	var slot *sixelSlot
	for _, w := range wants {
		k := w.artKey
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
		case art.ModeSixel:
			if s, ok := a.sixels[k]; ok {
				if slot == nil {
					slot = &sixelSlot{key: k, x: w.x, y: w.y, img: s}
				}
				continue
			}
			if a.encoding[k] {
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
		case art.ModeSixel:
			a.encoding[k] = true
			cmds = append(cmds, encodeSixel(img, k, a.cellSize()))
		}
	}
	if a.mode == art.ModeSixel {
		cmds = append(cmds, a.place(slot))
	}
	return tea.Batch(cmds...)
}

// place makes s the sixel image of the frame. A new image is drawn
// sixelDelay later, after the renderer wrote the empty box for it.
func (a *artState) place(s *sixelSlot) tea.Cmd {
	old := a.slot
	if s == nil {
		if old != nil {
			// The frame no longer has the marked box. The renderer writes
			// over it, which erases the image.
			a.slot = nil
			a.drawGen++
		}
		return nil
	}
	if old != nil && old.key == s.key && old.x == s.x && old.y == s.y {
		return nil
	}
	// A new image in the same box is drawn over the old one. If it covers
	// different pixels, the marks change, so the renderer erases the box
	// first.
	if old != nil && old.x == s.x && old.y == s.y && old.key.cols == s.key.cols && old.key.rows == s.key.rows &&
		(old.img.Offset != s.img.Offset || old.img.Size != s.img.Size) {
		a.parity = !a.parity
	}
	a.slot = s
	a.drawGen++
	gen := a.drawGen
	return tea.Tick(sixelDelay, func(time.Time) tea.Msg { return sixelDrawMsg{gen: gen} })
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
	case art.ModeSixel:
		if a.slot != nil && a.slot.key == k {
			return sixelBox(k.cols, k.rows, a.parity), true
		}
	case art.ModeBlocks:
		s, ok := a.blocks[k]
		return s, ok
	}
	return "", false
}

// sixelBox is the text under a sixel image: spaces with marks that the
// renderer keeps track of but that do not show. Conceal stops the renderer
// from erasing the spaces with an erase sequence. A different color on
// each row stops it from scrolling rows, which would move the image.
// Italic follows parity, so a flip makes the renderer write the box again.
func sixelBox(cols, rows int, parity bool) string {
	italic := ""
	if parity {
		italic = "3;"
	}
	var b strings.Builder
	for r := range rows {
		if r > 0 {
			b.WriteByte('\n')
		}
		fmt.Fprintf(&b, "\x1b[%s8;38;5;%dm%s\x1b[m", italic, 16+r%240, strings.Repeat(" ", cols))
	}
	return b.String()
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

// encodeSixel encodes the image off the UI loop.
func encodeSixel(img image.Image, k artKey, cell image.Point) tea.Cmd {
	return func() tea.Msg {
		s, err := art.Sixel(img, k.cols, k.rows, cell.X, cell.Y)
		return sixelEncodedMsg{key: k, cell: cell, img: s, err: err}
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
func (m *Model) artWants() []artWant {
	var wants []artWant
	if m.showPanel() {
		width, cols, rows, _ := m.panelLayout()
		if url := m.panelSubject().image; url != "" {
			// The panel is at the right edge, and its art box is at the top
			// of the body.
			wants = append(wants, artWant{artKey{url: url, cols: cols, rows: rows}, m.width - width, bodyTop})
		}
	}
	if url := m.thumbURL(); url != "" {
		wants = append(wants, artWant{artKey: artKey{url: url, cols: thumbCols, rows: thumbRows}})
	}
	return wants
}

// thumbURL is the cover of the playing track for the now-playing bar. Only
// kitty graphics draw it: half blocks are not sharp enough at this size,
// and sixel mode draws only the panel.
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
