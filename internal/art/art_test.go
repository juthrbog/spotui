package art

import (
	"bytes"
	"context"
	"image"
	"image/color"
	"image/png"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/charmbracelet/x/ansi"
	"github.com/charmbracelet/x/ansi/kitty"
)

func solid(w, h int, c color.Color) image.Image {
	img := image.NewRGBA(image.Rect(0, 0, w, h))
	for y := range h {
		for x := range w {
			img.Set(x, y, c)
		}
	}
	return img
}

func TestBlocksFillsTheBox(t *testing.T) {
	out := Blocks(solid(300, 300, color.RGBA{R: 200, G: 10, B: 30, A: 255}), 20, 10)
	lines := strings.Split(out, "\n")
	if len(lines) != 10 {
		t.Fatalf("%d lines, want 10", len(lines))
	}
	for i, l := range lines {
		if w := ansi.StringWidth(l); w != 20 {
			t.Errorf("line %d is %d cells, want 20", i, w)
		}
	}
	if !strings.Contains(out, "38;2;200;10;30;48;2;200;10;30m▀") {
		t.Errorf("output does not use the image color: %q", lines[0][:60])
	}
}

func TestBlocksKeepsAspectRatio(t *testing.T) {
	// A wide image in a square box leaves empty rows at the top and bottom.
	lines := strings.Split(Blocks(solid(400, 100, color.White), 10, 5), "\n")
	if strings.Contains(lines[0], "▀") || strings.Contains(lines[0], "▄") {
		t.Errorf("top row is not empty: %q", lines[0])
	}
	if !strings.Contains(lines[2], "▀") {
		t.Errorf("middle row is empty: %q", lines[2])
	}
}

func TestKittyPlaceholderGrid(t *testing.T) {
	out := KittyPlaceholder(42, 3, 2)
	lines := strings.Split(out, "\n")
	if len(lines) != 2 {
		t.Fatalf("%d lines, want 2", len(lines))
	}
	if !strings.HasPrefix(lines[1], "\x1b[38;5;42m") {
		t.Errorf("row does not set the image id color: %q", lines[1])
	}
	want := string([]rune{kitty.Placeholder, kitty.Diacritic(1), kitty.Diacritic(2)})
	if !strings.Contains(lines[1], want) {
		t.Error("last cell does not carry row 1 and column 2")
	}
	if w := ansi.StringWidth(lines[0]); w != 3 {
		t.Errorf("row is %d cells, want 3", w)
	}
}

func TestKittyTransmitUsesVirtualPlacement(t *testing.T) {
	seq, err := KittyTransmit(solid(640, 640, color.Black), 20, 8, 4)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.HasPrefix(seq, "\x1b_G") {
		t.Fatalf("not a kitty graphics sequence: %q", seq[:20])
	}
	first := seq[:strings.Index(seq, ";")]
	for _, opt := range []string{"a=T", "i=20", "U=1", "c=8", "r=4", "f=100", "q=2"} {
		if !strings.Contains(first, opt) {
			t.Errorf("first chunk options %q lack %s", first, opt)
		}
	}
}

func TestFetchRejectsBadInput(t *testing.T) {
	var pngData bytes.Buffer
	if err := png.Encode(&pngData, solid(4, 4, color.White)); err != nil {
		t.Fatal(err)
	}
	srv := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/ok.png":
			_, _ = w.Write(pngData.Bytes())
		case "/text":
			_, _ = w.Write([]byte("not an image"))
		default:
			http.NotFound(w, r)
		}
	}))
	defer srv.Close()
	ctx := context.Background()

	img, err := Fetch(ctx, srv.Client(), srv.URL+"/ok.png")
	if err != nil || img.Bounds().Dx() != 4 {
		t.Fatalf("ok.png: img=%v err=%v", img, err)
	}
	if _, err := Fetch(ctx, srv.Client(), srv.URL+"/text"); err == nil {
		t.Error("text: no error")
	}
	if _, err := Fetch(ctx, srv.Client(), srv.URL+"/missing"); err == nil {
		t.Error("missing: no error")
	}
	if _, err := Fetch(ctx, srv.Client(), "http://example.com/a.jpg"); err == nil {
		t.Error("http URL: no error")
	}
}

func TestFitRect(t *testing.T) {
	cases := []struct {
		src  image.Rectangle
		w, h int
		want image.Rectangle
	}{
		{image.Rect(0, 0, 300, 300), 20, 20, image.Rect(0, 0, 20, 20)},
		{image.Rect(0, 0, 400, 100), 10, 10, image.Rect(0, 4, 10, 6)},
		{image.Rect(0, 0, 100, 400), 10, 10, image.Rect(4, 0, 6, 10)},
	}
	for _, c := range cases {
		if got := fitRect(c.src, c.w, c.h); got != c.want {
			t.Errorf("fitRect(%v, %d, %d) = %v, want %v", c.src, c.w, c.h, got, c.want)
		}
	}
}
