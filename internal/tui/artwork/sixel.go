package artwork

import (
	"bytes"
	"fmt"
	"image"
	"image/png"
	"io"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync"

	"github.com/charmbracelet/x/ansi"
	"github.com/charmbracelet/x/ansi/kitty"
	"github.com/charmbracelet/x/ansi/sixel"
	"github.com/ygelfand/plexctl/internal/termgraphics"
	"golang.org/x/image/draw"
)

// Sixel has no movable text placements. Reuse our placeholder cells internally
// for clipping/compositing, replace them with spaces at the output boundary,
// then draw only visible pixel rectangles after the text frame.
type sixelRenderer struct {
	sync.Mutex
	dir        string
	nextID     int
	views      map[string]string
	files      map[int]string
	frame      string
	last       string
	placements []imagePlacement
	encoded    map[imagePlacement]string
	cellW      int
	cellH      int
}

type imagePlacement struct {
	ID     int
	X, Y   int
	Source image.Rectangle // cells within the original image
}

func newSixelRenderer() (Renderer, error) {
	dir, err := os.MkdirTemp("", "plexctl-sixel-")
	if err != nil {
		return nil, err
	}
	caps := termgraphics.Current()
	cw, ch := caps.CellWidth, caps.CellHeight
	if cw <= 0 || ch <= 0 {
		cw, ch = 8, 16
	}
	return &sixelRenderer{dir: dir, nextID: 1, cellW: cw, cellH: ch,
		views: make(map[string]string), files: make(map[int]string), encoded: make(map[imagePlacement]string)}, nil
}

func (*sixelRenderer) Name() string { return "sixel" }

func (r *sixelRenderer) Cached(id string, width, height int) (string, bool) {
	r.Lock()
	defer r.Unlock()
	view, ok := r.views[key(id, width, height)]
	return view, ok
}

func (r *sixelRenderer) Render(id string, img image.Image, width, height int) (string, error) {
	r.Lock()
	defer r.Unlock()
	cacheKey := key(id, width, height)
	if view, ok := r.views[cacheKey]; ok {
		return view, nil
	}
	cols, rows := fitImageWithCells(img.Bounds(), width, height, r.cellW, r.cellH)
	scaled := image.NewRGBA(image.Rect(0, 0, cols*r.cellW, rows*r.cellH))
	draw.ApproxBiLinear.Scale(scaled, scaled.Bounds(), img, img.Bounds(), draw.Src, nil)
	imageID := r.nextID
	r.nextID++
	path := filepath.Join(r.dir, strconv.Itoa(imageID))
	f, err := os.Create(path)
	if err != nil {
		return "", err
	}
	encodeErr := png.Encode(f, scaled)
	closeErr := f.Close()
	if encodeErr != nil {
		return "", encodeErr
	}
	if closeErr != nil {
		return "", closeErr
	}
	view := padImage(placeholders(imageID, cols, rows), width, height)
	r.files[imageID], r.views[cacheKey] = path, view
	return view, nil
}

func (r *sixelRenderer) Frame(view string) string {
	r.Lock()
	defer r.Unlock()
	r.frame = view
	r.placements = scanImages(view, r.files)
	return view
}

var diacriticIndex = func() map[rune]int {
	result := make(map[rune]int)
	for i := range 256 {
		result[kitty.Diacritic(i)] = i
	}
	return result
}()

func scanImages(view string, files map[int]string) []imagePlacement {
	placements := []imagePlacement{}
	x, y, foreground := 0, 0, 0
	var state byte
	for len(view) > 0 {
		sequence, width, n, next := ansi.DecodeSequence(view, state, nil)
		if n == 0 {
			break
		}
		view, state = view[n:], next
		if sequence == "\n" {
			x, y = 0, y+1
			continue
		}
		if sequence == "\r" {
			x = 0
			continue
		}
		if strings.HasPrefix(sequence, "\x1b[") && strings.HasSuffix(sequence, "m") {
			foreground = sgrForeground(sequence, foreground)
		}
		runes := []rune(sequence)
		if len(runes) >= 3 && runes[0] == kitty.Placeholder && files[foreground] != "" {
			row, rowOK := diacriticIndex[runes[1]]
			col, colOK := diacriticIndex[runes[2]]
			if rowOK && colOK {
				cell := imagePlacement{ID: foreground, X: x, Y: y, Source: image.Rect(col, row, col+1, row+1)}
				if len(placements) > 0 {
					last := &placements[len(placements)-1]
					if last.ID == cell.ID && last.Y == y && last.X+last.Source.Dx() == x &&
						last.Source.Min.Y == row && last.Source.Max.X == col {
						last.Source.Max.X++
						x += width
						continue
					}
				}
				placements = append(placements, cell)
			}
		}
		x += width
	}
	// Merge matching horizontal runs into rectangles, including separate copies
	// of one image and pieces exposed beside or below an overlay.
	merged := []imagePlacement{}
	for _, run := range placements {
		joined := false
		for i := len(merged) - 1; i >= 0; i-- {
			previous := &merged[i]
			if previous.ID == run.ID && previous.X == run.X && previous.Y+previous.Source.Dy() == run.Y &&
				previous.Source.Min.X == run.Source.Min.X && previous.Source.Max.X == run.Source.Max.X &&
				previous.Source.Max.Y == run.Source.Min.Y {
				previous.Source.Max.Y++
				joined = true
				break
			}
		}
		if !joined {
			merged = append(merged, run)
		}
	}
	return merged
}

func sgrForeground(sequence string, previous int) int {
	values := strings.Split(strings.TrimSuffix(strings.TrimPrefix(sequence, "\x1b["), "m"), ";")
	for i := 0; i < len(values); i++ {
		switch values[i] {
		case "", "0", "39":
			previous = 0
		case "38":
			if i+4 < len(values) && values[i+1] == "2" {
				r, _ := strconv.Atoi(values[i+2])
				g, _ := strconv.Atoi(values[i+3])
				b, _ := strconv.Atoi(values[i+4])
				previous = r<<16 | g<<8 | b
				i += 4
			}
		case "48", "58":
			if i+1 < len(values) {
				switch values[i+1] {
				case "2":
					i += 4
				case "5":
					i += 2
				}
			}
		}
	}
	return previous
}

func withoutPlaceholders(text string) string {
	var result strings.Builder
	var state byte
	for len(text) > 0 {
		sequence, _, n, next := ansi.DecodeSequence(text, state, nil)
		if n == 0 {
			break
		}
		if strings.HasPrefix(sequence, string(kitty.Placeholder)) {
			result.WriteByte(' ')
		} else {
			result.WriteString(sequence)
		}
		text, state = text[n:], next
	}
	return result.String()
}

func (r *sixelRenderer) Write(out io.Writer, p []byte) (int, error) {
	r.Lock()
	defer r.Unlock()
	text := string(p)
	if strings.Contains(text, "\x1b[?1049l") {
		r.last = ""
		return out.Write(p)
	}
	if strings.Contains(text, "\x1b[2J") {
		r.last = ""
	}
	var frame bytes.Buffer
	frame.WriteString("\x1b[?2026h")
	frame.WriteString(withoutPlaceholders(text))
	if r.frame != "" && r.frame != r.last {
		// Repaint changed text rows before images. This also erases old Sixels
		// when scrolling or opening dialogs, including partially visible bands.
		frame.WriteString("\x1b7")
		old, lines := strings.Split(r.last, "\n"), strings.Split(r.frame, "\n")
		changed := make(map[int]bool)
		for y, line := range lines {
			if y < len(old) && old[y] == line {
				continue
			}
			changed[y] = true
			fmt.Fprintf(&frame, "\x1b[%d;1H\x1b[0m\x1b[2K%s", y+1, withoutPlaceholders(line))
		}
		for _, placement := range r.placements {
			dirty := false
			for y := placement.Y; y < placement.Y+placement.Source.Dy(); y++ {
				dirty = dirty || changed[y]
			}
			if !dirty {
				continue
			}
			encoded, err := r.encode(placement)
			if err != nil {
				return 0, err
			}
			fmt.Fprintf(&frame, "\x1b[%d;%dH%s", placement.Y+1, placement.X+1, termgraphics.Wrap(encoded))
		}
		frame.WriteString("\x1b8")
	}
	frame.WriteString("\x1b[?2026l")
	if _, err := out.Write(frame.Bytes()); err != nil {
		return 0, err
	}
	r.last = r.frame
	return len(p), nil
}

func (r *sixelRenderer) encode(placement imagePlacement) (string, error) {
	// Placement coordinates do not affect the encoded pixels.
	placement.X, placement.Y = 0, 0
	if encoded, ok := r.encoded[placement]; ok {
		return encoded, nil
	}
	f, err := os.Open(r.files[placement.ID])
	if err != nil {
		return "", err
	}
	img, err := png.Decode(f)
	_ = f.Close()
	if err != nil {
		return "", err
	}
	cells := placement.Source
	bounds := image.Rect(cells.Min.X*r.cellW, cells.Min.Y*r.cellH, cells.Max.X*r.cellW, cells.Max.Y*r.cellH)
	// Normalize to (0,0): the Sixel encoder indexes pixels from the origin.
	crop := image.NewRGBA(image.Rect(0, 0, bounds.Dx(), bounds.Dy()))
	draw.Draw(crop, crop.Bounds(), img, bounds.Min, draw.Src)
	var encoded bytes.Buffer
	encoded.WriteString("\x1bP0;1q") // Transparent background prevents band padding from erasing adjacent text.
	if err := (&sixel.Encoder{}).Encode(&encoded, crop); err != nil {
		return "", err
	}
	encoded.WriteString("\x1b\\")
	if len(r.encoded) >= 64 {
		clear(r.encoded)
	}
	r.encoded[placement] = encoded.String()
	return encoded.String(), nil
}

func (r *sixelRenderer) Close(io.Writer) error {
	r.Lock()
	defer r.Unlock()
	clear(r.files)
	clear(r.views)
	clear(r.encoded)
	r.frame, r.last = "", ""
	if r.dir == "" {
		return nil
	}
	err := os.RemoveAll(r.dir)
	r.dir = ""
	return err
}
