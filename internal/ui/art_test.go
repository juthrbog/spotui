package ui

import (
	"context"
	"image"
	"path/filepath"
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"
	uv "github.com/charmbracelet/ultraviolet"
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

func TestDetectKittySupport(t *testing.T) {
	t.Setenv("TERM_PROGRAM", "ghostty")
	t.Setenv("KONSOLE_VERSION", "")
	cases := []struct {
		name string
		msg  tea.Msg
		want art.Mode
	}{
		{"terminal says OK", uv.KittyGraphicsEvent{Options: kitty.Options{ID: art.QueryID}, Payload: []byte("OK")}, art.ModeKitty},
		{"terminal says no", uv.KittyGraphicsEvent{Options: kitty.Options{ID: art.QueryID}, Payload: []byte("ENOTSUPPORTED")}, art.ModeBlocks},
		{"no answer", artDetectTimeoutMsg{}, art.ModeBlocks},
	}
	for _, c := range cases {
		a := newArtState("auto")
		if !a.detecting || a.init() == nil {
			t.Fatalf("%s: auto does not start detection", c.name)
		}
		a.update(c.msg)
		if a.detecting || a.mode != c.want {
			t.Errorf("%s: mode = %v (detecting %v), want %v", c.name, a.mode, a.detecting, c.want)
		}
	}

	t.Setenv("TERM_PROGRAM", "WezTerm")
	a := newArtState("auto")
	a.update(uv.KittyGraphicsEvent{Options: kitty.Options{ID: art.QueryID}, Payload: []byte("OK")})
	if a.mode != art.ModeBlocks {
		t.Errorf("WezTerm: mode = %v, want blocks", a.mode)
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
