package ui

import (
	"context"
	"image"
	"path/filepath"
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"
	uv "github.com/charmbracelet/ultraviolet"
	"github.com/charmbracelet/x/ansi"
	"github.com/charmbracelet/x/ansi/kitty"
	"github.com/devgianlu/go-librespot/daemon"

	"github.com/juthrbog/spotui/internal/art"
	"github.com/juthrbog/spotui/internal/engine"
	"github.com/juthrbog/spotui/internal/webapi"
)

const coverURL = "https://i.scdn.co/image/cover"

func artModel(t *testing.T, setting string, w, h int) *Model {
	t.Helper()
	openBrowser = func(string) {}
	web := webapi.New("", 8898, filepath.Join(t.TempDir(), "token.json"))
	m := New(context.Background(), engine.New(engine.Options{}), web, Info{CoverArt: setting})
	m.Update(tea.WindowSizeMsg{Width: w, Height: h})
	track := &daemon.ApiTrack{Uri: "spotify:track:1", Name: "Song", ArtistNames: []string{"Band"}, AlbumCoverUrl: ptr(coverURL), Duration: 1000}
	m.Update(statusMsg{st: &daemon.ApiStatus{Username: "jakob", Track: track, VolumeSteps: 100}})
	m.Update(playlistsMsg{items: []daemon.ApiLibraryPlaylist{{Name: "Mix", Uri: "spotify:playlist:1", ImageUrl: ptr(coverURL)}}})
	m.Update(tea.KeyPressMsg{Code: tea.KeyDown}) // from Liked Songs to Mix
	return m
}

func ptr(s string) *string { return &s }

func cover() image.Image {
	img := image.NewRGBA(image.Rect(0, 0, 64, 64))
	for i := range img.Pix {
		img.Pix[i] = 200
	}
	return img
}

func TestDetectImageSupport(t *testing.T) {
	t.Setenv("TERM_PROGRAM", "ghostty")
	t.Setenv("KONSOLE_VERSION", "")
	kittyOK := uv.KittyGraphicsEvent{Options: kitty.Options{ID: art.QueryID}, Payload: []byte("OK")}
	kittyNo := uv.KittyGraphicsEvent{Options: kitty.Options{ID: art.QueryID}, Payload: []byte("ENOTSUPPORTED")}
	da1Sixel := uv.PrimaryDeviceAttributesEvent{61, 4, 6, 7, 22}
	da1Plain := uv.PrimaryDeviceAttributesEvent{62, 22}
	cases := []struct {
		name      string
		sixelAuto bool
		msgs      []tea.Msg
		want      art.Mode
	}{
		{"kitty says OK", true, []tea.Msg{kittyOK, da1Sixel}, art.ModeKitty},
		{"kitty says no, no sixel", true, []tea.Msg{kittyNo, da1Plain}, art.ModeBlocks},
		{"kitty says no, sixel", true, []tea.Msg{kittyNo, da1Sixel}, art.ModeSixel},
		{"only DA1, sixel", true, []tea.Msg{da1Sixel}, art.ModeSixel},
		{"sixel off for this system", false, []tea.Msg{da1Sixel}, art.ModeBlocks},
		{"no answer", true, []tea.Msg{artDetectTimeoutMsg{}}, art.ModeBlocks},
	}
	defer func(v bool) { sixelAuto = v }(sixelAuto)
	for _, c := range cases {
		sixelAuto = c.sixelAuto
		a := newArtState("auto")
		if !a.detecting || a.init() == nil {
			t.Fatalf("%s: auto does not start detection", c.name)
		}
		for _, msg := range c.msgs {
			if ok, _ := a.update(msg); !ok {
				t.Errorf("%s: %T not handled", c.name, msg)
			}
		}
		if a.detecting || a.mode != c.want {
			t.Errorf("%s: mode = %v (detecting %v), want %v", c.name, a.mode, a.detecting, c.want)
		}
	}

	// WezTerm answers the kitty query but cannot show placeholders.
	t.Setenv("TERM_PROGRAM", "WezTerm")
	sixelAuto = true
	a := newArtState("auto")
	a.update(kittyOK)
	if !a.detecting {
		t.Errorf("WezTerm: mode %v picked before DA1", a.mode)
	}
	a.update(da1Sixel)
	if a.mode != art.ModeSixel {
		t.Errorf("WezTerm: mode = %v, want sixel", a.mode)
	}
}

func TestSixelPanel(t *testing.T) {
	m := artModel(t, "sixel", 120, 36)
	width, cols, rows, ok := m.panelLayout()
	if !ok {
		t.Fatal("no panel at 120x36")
	}
	key := artKey{url: coverURL, cols: cols, rows: rows}
	m.Update(uv.CellSizeEvent{Width: 9, Height: 18})
	m.Update(artLoadedMsg{url: coverURL, img: cover()})
	if !m.art.encoding[key] {
		t.Fatal("encoding did not start")
	}
	if m.art.slot != nil {
		t.Fatal("sixel placed before it was encoded")
	}

	// An image made for an old cell size is dropped.
	old, err := art.Sixel(cover(), cols, rows, 10, 20)
	if err != nil {
		t.Fatal(err)
	}
	m.Update(sixelEncodedMsg{key: key, cell: image.Pt(10, 20), img: old})
	if m.art.slot != nil {
		t.Fatal("placed an image with the old cell size")
	}

	img, err := art.Sixel(cover(), cols, rows, 9, 18)
	if err != nil {
		t.Fatal(err)
	}
	_, cmd := m.Update(sixelEncodedMsg{key: key, cell: image.Pt(9, 18), img: img})
	if m.art.slot == nil || cmd == nil {
		t.Fatal("encoded image was not placed")
	}

	// The frame has the marked box exactly where the image goes.
	frame := checkFrame(t, m, 120, 36, "sixel panel")
	lines := strings.Split(frame, "\n")
	for r := range rows {
		line := lines[bodyTop+r]
		i := strings.Index(line, "\x1b[8;38;5;")
		if i < 0 {
			t.Fatalf("row %d of the box has no marks: %q", r, ansi.Strip(line))
		}
		if x := ansi.StringWidth(line[:i]); x != 120-width {
			t.Errorf("row %d of the box starts at column %d, want %d", r, x, 120-width)
		}
	}
	if strings.Contains(frame, "▀") || strings.ContainsRune(frame, kitty.Placeholder) {
		t.Error("sixel mode drew blocks or kitty placeholders")
	}

	// Only the latest scheduled draw goes out.
	gen := m.art.drawGen
	_, cmd = m.Update(sixelDrawMsg{gen: gen - 1})
	if raw, ok := runCmd(cmd).(tea.RawMsg); ok {
		t.Errorf("old draw went out: %q", raw.Msg)
	}
	_, cmd = m.Update(sixelDrawMsg{gen: gen})
	raw, ok := runCmd(cmd).(tea.RawMsg)
	if !ok {
		t.Fatal("draw sent nothing")
	}
	want := ansi.SaveCursor + ansi.CursorPosition(120-width+1, bodyTop+1) + "\x1bP0;1q"
	if s := raw.Msg.(string); !strings.HasPrefix(s, want) || !strings.HasSuffix(s, ansi.RestoreCursor) {
		t.Errorf("draw = %q…, want prefix %q", s[:min(len(s), 40)], want)
	}

	// The help screen covers the panel, which erases the image. The image
	// comes back when the help screen closes.
	m.Update(tea.KeyPressMsg{Code: '?', Text: "?"})
	if m.art.slot != nil {
		t.Error("image still placed under the help screen")
	}
	_, cmd = m.Update(tea.KeyPressMsg{Code: tea.KeyEscape})
	if m.art.slot == nil || cmd == nil {
		t.Error("image not drawn again after the help screen")
	}

	// A resize erases the screen and changes the box, so the image is
	// encoded again for the new box.
	m.Update(tea.WindowSizeMsg{Width: 130, Height: 40})
	_, cols, rows, _ = m.panelLayout()
	if m.art.slot != nil || !m.art.encoding[artKey{url: coverURL, cols: cols, rows: rows}] {
		t.Errorf("after resize: slot %v, encoding %v", m.art.slot, m.art.encoding)
	}
}

// runCmd runs cmd and returns the first message that is not a batch.
func runCmd(cmd tea.Cmd) tea.Msg {
	if cmd == nil {
		return nil
	}
	msg := cmd()
	if batch, ok := msg.(tea.BatchMsg); ok {
		for _, c := range batch {
			if m := runCmd(c); m != nil {
				return m
			}
		}
		return nil
	}
	return msg
}

func TestSixelBoxMarks(t *testing.T) {
	even := strings.Split(sixelBox(8, 4, false), "\n")
	odd := strings.Split(sixelBox(8, 4, true), "\n")
	if len(even) != 4 {
		t.Fatalf("%d rows, want 4", len(even))
	}
	for r := range even {
		if w := ansi.StringWidth(even[r]); w != 8 {
			t.Errorf("row %d is %d cells, want 8", r, w)
		}
		if strings.TrimSpace(ansi.Strip(even[r])) != "" {
			t.Errorf("row %d shows text: %q", r, ansi.Strip(even[r]))
		}
		if even[r] == odd[r] {
			t.Errorf("row %d does not change with parity", r)
		}
		// Equal rows let the renderer scroll them, which moves the image.
		if r > 0 && even[r] == even[r-1] {
			t.Errorf("rows %d and %d are equal", r-1, r)
		}
	}
}

func TestSixelSwapRedrawsOnlyWhenNeeded(t *testing.T) {
	a := newArtState("sixel")
	square := art.SixelImage{Seq: "sq", Size: image.Pt(40, 40)}
	wide := art.SixelImage{Seq: "wide", Offset: image.Pt(0, 1), Size: image.Pt(40, 10)}
	at := func(url string, img art.SixelImage) *sixelSlot {
		return &sixelSlot{key: artKey{url: url, cols: 4, rows: 2}, x: 50, y: 3, img: img}
	}

	if a.place(at("a", square)) == nil {
		t.Fatal("first image not scheduled")
	}
	if a.place(at("a", square)) != nil {
		t.Error("same image scheduled again")
	}
	parity := a.parity
	if a.place(at("b", square)) == nil || a.parity != parity {
		t.Error("image with the same size: want a draw over the old one, without an erase")
	}
	if a.place(at("c", wide)) == nil || a.parity == parity {
		t.Error("image with another size: want an erase and a draw")
	}
	gen := a.drawGen
	if a.place(nil) != nil || a.slot != nil || a.drawGen == gen {
		t.Error("removing the image did not cancel the draw")
	}
}

func TestBlocksPanelShowsSelectedCover(t *testing.T) {
	m := artModel(t, "blocks", 120, 36)
	if sel := m.current().selection(); sel == nil || sel.image != coverURL {
		t.Fatalf("selection = %+v, want the Mix playlist", sel)
	}
	if got := checkFrame(t, m, 120, 36, "panel loading"); strings.Contains(got, "▀") {
		t.Error("cover drawn before it was downloaded")
	}
	if !m.art.loading[coverURL] {
		t.Fatal("cover download did not start")
	}

	m.Update(artLoadedMsg{url: coverURL, img: cover()})
	frame := checkFrame(t, m, 120, 36, "panel with cover")
	if !strings.Contains(frame, "▀") {
		t.Error("panel does not show the cover")
	}
	if !strings.Contains(frame, "│") {
		t.Error("panel has no separator")
	}
	if strings.ContainsRune(frame, kitty.Placeholder) {
		t.Error("blocks mode drew kitty placeholders")
	}
}

func TestKittyPanelAndThumbnail(t *testing.T) {
	m := artModel(t, "kitty", 120, 36)
	m.Update(artLoadedMsg{url: coverURL, img: cover()})

	_, cols, rows, ok := m.panelLayout()
	if !ok {
		t.Fatal("no panel at 120x36")
	}
	if _, ok := m.art.ids[artKey{url: coverURL, cols: cols, rows: rows}]; !ok {
		t.Error("panel cover was not sent to the terminal")
	}
	if _, ok := m.art.ids[artKey{url: coverURL, cols: thumbCols, rows: thumbRows}]; !ok {
		t.Error("now-playing thumbnail was not sent to the terminal")
	}
	frame := checkFrame(t, m, 120, 36, "kitty panel")
	if n := strings.Count(frame, string(kitty.Placeholder)); n != cols*rows+thumbCols*thumbRows {
		t.Errorf("%d placeholder cells, want %d", n, cols*rows+thumbCols*thumbRows)
	}
	if got := m.art.cleanup(); strings.Count(got, "a=d") != 2 {
		t.Errorf("cleanup deletes %d images, want 2", strings.Count(got, "a=d"))
	}
}

func TestNoPanelWhenNarrowOrOff(t *testing.T) {
	if m := artModel(t, "blocks", 80, 30); m.showPanel() {
		t.Error("panel shown at 80 columns")
	}
	m := artModel(t, "off", 120, 36)
	if m.showPanel() {
		t.Error("panel shown with cover_art off")
	}
	if m.art.loading[coverURL] {
		t.Error("cover downloaded with cover_art off")
	}
}

func TestKittyIDsAreReused(t *testing.T) {
	a := newArtState("kitty")
	total := lastKittyID - firstKittyID + 1
	for i := range total {
		id, evicted := a.allocID(artKey{url: string(rune('a' + i)), cols: 1, rows: 1})
		if evicted || id != firstKittyID+i {
			t.Fatalf("alloc %d: id %d evicted %v", i, id, evicted)
		}
	}
	// Showing the first image again keeps it from being reused next.
	first := artKey{url: "a", cols: 1, rows: 1}
	a.touchID(first)
	id, evicted := a.allocID(artKey{url: "new", cols: 1, rows: 1})
	if !evicted || id != firstKittyID+1 {
		t.Errorf("full table: id %d evicted %v, want id %d reused", id, evicted, firstKittyID+1)
	}
	if _, ok := a.ids[first]; !ok {
		t.Error("recently shown image lost its id")
	}
}
