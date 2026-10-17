// Package art downloads cover images and turns them into terminal text. It
// supports three ways to draw an image:
//
//   - Kitty graphics with Unicode placeholders. The image is sent to the
//     terminal once, and the screen shows it through special placeholder
//     characters. This gives a sharp image in kitty, Ghostty, and terminals
//     that copy their protocol.
//   - Sixel. The image is drawn in pixels at the cursor, outside the text
//     of the screen. This gives a sharp image in Windows Terminal, which
//     does not support kitty graphics.
//   - Half blocks. Each cell shows two pixels with the "▀" character, colored
//     with the top pixel in front and the bottom pixel behind. This works in
//     any terminal with true color.
package art

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"image"
	_ "image/jpeg" // Spotify covers are JPEG.
	_ "image/png"
	"io"
	"net/http"
	"os"
	"slices"
	"strconv"
	"strings"

	"github.com/charmbracelet/x/ansi"
	"github.com/charmbracelet/x/ansi/kitty"
	"github.com/charmbracelet/x/ansi/sixel"
	"golang.org/x/image/draw"
)

// Mode is how images are drawn.
type Mode int

const (
	ModeOff Mode = iota
	ModeBlocks
	ModeKitty
	ModeSixel
)

func (m Mode) String() string {
	switch m {
	case ModeBlocks:
		return "blocks"
	case ModeKitty:
		return "kitty"
	case ModeSixel:
		return "sixel"
	default:
		return "off"
	}
}

const (
	maxImageBytes = 5 << 20
	maxDimension  = 4096
	// maxKittyPixels caps the size of an image sent to the terminal. The
	// terminal scales it to the cell box.
	maxKittyPixels = 400
	// maxSixelPixels caps the size of a sixel box. The terminal does not
	// scale sixel images, so the image has the pixel size of the box.
	maxSixelPixels = 2048
)

// Fetch downloads and decodes the image at url.
func Fetch(ctx context.Context, client *http.Client, url string) (image.Image, error) {
	if !strings.HasPrefix(url, "https://") {
		return nil, errors.New("cover art URL does not use https")
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return nil, err
	}
	resp, err := client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("downloading cover art: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("downloading cover art: HTTP %d", resp.StatusCode)
	}
	data, err := io.ReadAll(io.LimitReader(resp.Body, maxImageBytes+1))
	if err != nil {
		return nil, fmt.Errorf("downloading cover art: %w", err)
	}
	if len(data) > maxImageBytes {
		return nil, errors.New("cover art is too large")
	}
	return decode(data)
}

func decode(data []byte) (image.Image, error) {
	// Read the size first, so a hostile image cannot use all memory.
	cfg, _, err := image.DecodeConfig(bytes.NewReader(data))
	if err != nil {
		return nil, fmt.Errorf("reading cover art: %w", err)
	}
	if cfg.Width <= 0 || cfg.Height <= 0 || cfg.Width > maxDimension || cfg.Height > maxDimension {
		return nil, fmt.Errorf("cover art has unusual size %dx%d", cfg.Width, cfg.Height)
	}
	img, _, err := image.Decode(bytes.NewReader(data))
	if err != nil {
		return nil, fmt.Errorf("reading cover art: %w", err)
	}
	return img, nil
}

// fitRect returns the largest rectangle with the aspect ratio of src that
// fits in a w×h box, centered in the box.
func fitRect(src image.Rectangle, w, h int) image.Rectangle {
	sw, sh := src.Dx(), src.Dy()
	if sw <= 0 || sh <= 0 {
		return image.Rectangle{}
	}
	fw, fh := w, sh*w/sw
	if fh > h {
		fw, fh = sw*h/sh, h
	}
	fw, fh = max(fw, 1), max(fh, 1)
	x, y := (w-fw)/2, (h-fh)/2
	return image.Rect(x, y, x+fw, y+fh)
}

// Blocks draws img in a cols×rows cell box with half blocks. The image keeps
// its aspect ratio, and the unused part of the box stays empty.
func Blocks(img image.Image, cols, rows int) string {
	if cols <= 0 || rows <= 0 {
		return ""
	}
	dst := image.NewRGBA(image.Rect(0, 0, cols, rows*2))
	draw.CatmullRom.Scale(dst, fitRect(img.Bounds(), cols, rows*2), img, img.Bounds(), draw.Src, nil)

	var b strings.Builder
	for y := range rows {
		if y > 0 {
			b.WriteByte('\n')
		}
		for x := range cols {
			top, bottom := dst.RGBAAt(x, 2*y), dst.RGBAAt(x, 2*y+1)
			switch {
			case top.A == 0 && bottom.A == 0:
				b.WriteString("\x1b[m ")
			case bottom.A == 0:
				fmt.Fprintf(&b, "\x1b[m\x1b[38;2;%d;%d;%dm▀", top.R, top.G, top.B)
			case top.A == 0:
				fmt.Fprintf(&b, "\x1b[m\x1b[38;2;%d;%d;%dm▄", bottom.R, bottom.G, bottom.B)
			default:
				fmt.Fprintf(&b, "\x1b[38;2;%d;%d;%d;48;2;%d;%d;%dm▀",
					top.R, top.G, top.B, bottom.R, bottom.G, bottom.B)
			}
		}
		b.WriteString("\x1b[m")
	}
	return b.String()
}

// KittyTransmit returns the escape sequence that sends img to the terminal
// as image id, with a virtual placement of cols×rows cells. Show the image
// with KittyPlaceholder.
func KittyTransmit(img image.Image, id, cols, rows int) (string, error) {
	if b := img.Bounds(); b.Dx() > maxKittyPixels || b.Dy() > maxKittyPixels {
		r := fitRect(b, maxKittyPixels, maxKittyPixels)
		small := image.NewRGBA(image.Rect(0, 0, r.Dx(), r.Dy()))
		draw.CatmullRom.Scale(small, small.Bounds(), img, b, draw.Src, nil)
		img = small
	}
	var buf strings.Builder
	err := kitty.EncodeGraphics(&buf, img, &kitty.Options{
		Action:           kitty.TransmitAndPut,
		Transmission:     kitty.Direct,
		Format:           kitty.PNG,
		ID:               id,
		Quiet:            2,
		Chunk:            true,
		VirtualPlacement: true,
		DoNotMoveCursor:  true,
		Columns:          cols,
		Rows:             rows,
	})
	if err != nil {
		return "", fmt.Errorf("encoding cover art: %w", err)
	}
	return buf.String(), nil
}

// KittyPlaceholder returns the text that shows image id in a cols×rows cell
// box. The foreground color carries the image id, and two diacritics on each
// cell carry its row and column. id must be from 16 to 255.
func KittyPlaceholder(id, cols, rows int) string {
	var b strings.Builder
	for r := range rows {
		if r > 0 {
			b.WriteByte('\n')
		}
		b.WriteString("\x1b[38;5;" + strconv.Itoa(id) + "m")
		for c := range cols {
			b.WriteRune(kitty.Placeholder)
			b.WriteRune(kitty.Diacritic(r))
			b.WriteRune(kitty.Diacritic(c))
		}
		b.WriteString("\x1b[39m")
	}
	return b.String()
}

// KittyDelete returns the escape sequence that frees image id in the
// terminal.
func KittyDelete(id int) string {
	return ansi.KittyGraphics(nil, "a=d", "d=I", "i="+strconv.Itoa(id), "q=2")
}

// QueryID is the image id of the support query.
const QueryID = 31

// KittyQuery returns a sequence that asks the terminal whether it supports
// kitty graphics. A terminal with support answers with image id QueryID and
// the payload "OK". Other terminals ignore it.
func KittyQuery() string {
	return ansi.KittyGraphics([]byte("AAAA"), "i="+strconv.Itoa(QueryID), "s=1", "v=1", "a=q", "t=d", "f=24")
}

// SixelImage is an image encoded as sixel for a box of cells.
type SixelImage struct {
	Seq    string      // draws the image at the cursor
	Offset image.Point // cell in the box where the image starts
	Size   image.Point // size of the image in pixels
}

// Sixel encodes img for a cols×rows cell box with cells of cellW×cellH
// pixels. The image keeps its aspect ratio and is centered in the box to
// the nearest cell.
func Sixel(img image.Image, cols, rows, cellW, cellH int) (SixelImage, error) {
	if cols <= 0 || rows <= 0 || cellW <= 0 || cellH <= 0 {
		return SixelImage{}, errors.New("empty sixel box")
	}
	w, h := cols*cellW, rows*cellH
	if w > maxSixelPixels || h > maxSixelPixels {
		return SixelImage{}, fmt.Errorf("sixel box of %dx%d pixels is too large", w, h)
	}
	r := fitRect(img.Bounds(), w, h)
	dst := image.NewRGBA(image.Rect(0, 0, r.Dx(), r.Dy()))
	draw.CatmullRom.Scale(dst, dst.Bounds(), img, img.Bounds(), draw.Src, nil)

	var buf bytes.Buffer
	if err := new(sixel.Encoder).Encode(&buf, dst); err != nil {
		return SixelImage{}, fmt.Errorf("encoding cover art: %w", err)
	}
	return SixelImage{
		// P2 = 1 leaves pixels without a color unchanged.
		Seq:    ansi.SixelGraphics(0, 1, 0, buf.Bytes()),
		Offset: image.Pt(r.Min.X/cellW, r.Min.Y/cellH),
		Size:   r.Size(),
	}, nil
}

// DrawAt returns the sequence that draws the image in the box with its
// top-left cell at column x and row y of the screen, counted from 0. It
// puts the cursor back where it was, so the screen renderer can go on
// from there.
func (s SixelImage) DrawAt(x, y int) string {
	return ansi.SaveCursor +
		ansi.CursorPosition(x+s.Offset.X+1, y+s.Offset.Y+1) +
		s.Seq +
		ansi.RestoreCursor
}

// CellSizeQuery returns a sequence that asks the terminal for the size of
// a cell in pixels.
func CellSizeQuery() string {
	return ansi.WindowOp(16)
}

// SixelSupported reports whether a primary device attributes (DA1) answer
// lists sixel graphics.
func SixelSupported(da1 []int) bool {
	return slices.Contains(da1, 4)
}

// PlaceholdersUnsupported reports whether the environment shows a terminal
// that answers the kitty query but cannot show Unicode placeholders.
func PlaceholdersUnsupported() bool {
	return os.Getenv("TERM_PROGRAM") == "WezTerm" || os.Getenv("KONSOLE_VERSION") != ""
}
