package termgraphics

import (
	"bytes"
	"fmt"
	"image"
	"io"
	"math/rand/v2"
	"strconv"
	"strings"
	"sync"
	"unicode/utf8"

	"github.com/charmbracelet/x/ansi"
)

// OverlayWriter confines mpv's cursor and text to a TUI content rectangle. It
// suppresses screen switching, clearing, input modes, and cursor visibility;
// those belong to Bubble Tea. Only complete escape sequences reach the output.
type OverlayWriter struct {
	mu             sync.Mutex
	output         io.Writer
	bounds         func() image.Rectangle
	graphics       Writer
	buffer         bytes.Buffer
	pending        []byte
	x, y           int
	foreground     string
	background     string
	hidden, closed bool
}

func NewOverlayWriter(output io.Writer, bounds func() image.Rectangle, kitty bool) *OverlayWriter {
	w := &OverlayWriter{output: output, bounds: bounds}
	w.graphics.Output = &w.buffer
	w.graphics.Direct = !InTmux()
	if kitty {
		w.graphics.ImageID = (1 << 30) + 2*rand.IntN(1<<19)
	}
	return w
}

func (w *OverlayWriter) SetVisible(visible bool) bool {
	w.mu.Lock()
	defer w.mu.Unlock()
	if w.hidden == !visible || w.closed {
		return false
	}
	w.hidden = !visible
	if w.hidden && w.graphics.ImageID != 0 {
		_, _ = io.WriteString(w.output, Wrap(w.graphics.DeleteImages()))
	}
	return true
}

func (w *OverlayWriter) Write(p []byte) (int, error) {
	w.mu.Lock()
	defer w.mu.Unlock()
	if w.closed {
		return len(p), nil
	}
	rect := w.bounds()
	w.pending = append(w.pending, p...)
	var translated strings.Builder
	fmt.Fprintf(&translated, "\x1b[%d;%dH%s%s", rect.Min.Y+w.y+1, rect.Min.X+w.x+1, w.foreground, w.background)
	prefixLen := translated.Len()
	for len(w.pending) > 0 {
		// DecodeSequence treats an ESC at the end of a string as cancellation.
		// Wait for ST ourselves so a write split between ESC and backslash
		// cannot truncate an APC/DCS graphics packet.
		graphics := bytes.HasPrefix(w.pending, []byte("\x1b_")) || bytes.HasPrefix(w.pending, []byte("\x1bP"))
		if graphics && !bytes.Contains(w.pending, []byte("\x1b\\")) {
			if len(w.pending) > 32<<20 {
				return 0, fmt.Errorf("overlay graphics packet exceeds 32 MiB")
			}
			break
		}
		if w.pending[0] >= 0x80 && !utf8.FullRune(w.pending) {
			break
		}
		sequence, width, n, state := ansi.DecodeSequence(w.pending, 0, nil)
		if state != 0 || n == 0 {
			if len(w.pending) > 32<<20 {
				return 0, fmt.Errorf("overlay graphics packet exceeds 32 MiB")
			}
			break
		}
		w.pending = w.pending[n:]
		s := string(sequence)
		switch {
		case strings.HasPrefix(s, "\x1b["):
			command := s[len(s)-1]
			args := strings.TrimSuffix(strings.TrimPrefix(s, "\x1b["), string(command))
			switch command {
			case 'H', 'f':
				row, col, _ := strings.Cut(args, ";")
				y, _ := strconv.Atoi(row)
				x, _ := strconv.Atoi(col)
				w.x, w.y = max(0, x-1), max(0, y-1)
			case 'A', 'B', 'C', 'D':
				n, _ := strconv.Atoi(args)
				n = max(1, n)
				switch command {
				case 'A':
					w.y = max(0, w.y-n)
				case 'B':
					w.y += n
				case 'C':
					w.x += n
				case 'D':
					w.x = max(0, w.x-n)
				}
			case 'm':
				translated.WriteString(s)
				w.colors(args)
				continue
			default:
				continue
			}
			fmt.Fprintf(&translated, "\x1b[%d;%dH", rect.Min.Y+w.y+1, rect.Min.X+w.x+1)
		case strings.HasPrefix(s, "\x1b_G"), strings.HasPrefix(s, "\x1bP"):
			if w.x < rect.Dx() && w.y < rect.Dy() {
				translated.WriteString(s)
			}
		case s == "\n":
			w.y++
			w.x = 0
			fmt.Fprintf(&translated, "\x1b[%d;%dH", rect.Min.Y+w.y+1, rect.Min.X+1)
		case s == "\r":
			w.x = 0
			fmt.Fprintf(&translated, "\x1b[%d;%dH", rect.Min.Y+w.y+1, rect.Min.X+1)
		case width > 0:
			if w.x+width <= rect.Dx() && w.y < rect.Dy() {
				translated.WriteString(s)
			}
			w.x += width
		}
	}
	if translated.Len() == prefixLen {
		return len(p), nil
	}
	w.buffer.Reset()
	if _, err := w.graphics.Write([]byte(translated.String())); err != nil {
		return 0, err
	}
	if !w.hidden && rect.Dx() > 0 && rect.Dy() > 0 {
		frame := "\x1b[?2026h\x1b7" + w.buffer.String() + "\x1b8\x1b[?2026l"
		if _, err := io.WriteString(w.output, frame); err != nil {
			return 0, err
		}
	}
	return len(p), nil
}

// TCT writes foreground and background separately; both must survive stdout
// chunk boundaries because saving/restoring the TUI cursor also restores SGR.
func (w *OverlayWriter) colors(args string) {
	fields := strings.Split(args, ";")
	for i := 0; i < len(fields); i++ {
		switch fields[i] {
		case "", "0":
			w.foreground, w.background = "", ""
		case "39":
			w.foreground = "\x1b[39m"
		case "49":
			w.background = "\x1b[49m"
		case "38", "48":
			n := 0
			if i+1 < len(fields) {
				switch fields[i+1] {
				case "2":
					n = 4
				case "5":
					n = 2
				}
			}
			if n == 0 || i+n >= len(fields) {
				continue
			}
			color := "\x1b[" + strings.Join(fields[i:i+n+1], ";") + "m"
			if fields[i] == "38" {
				w.foreground = color
			} else {
				w.background = color
			}
			i += n
		}
	}
}

func (w *OverlayWriter) Close() error {
	w.mu.Lock()
	defer w.mu.Unlock()
	if w.closed {
		return nil
	}
	w.closed = true
	if w.graphics.ImageID != 0 {
		_, err := io.WriteString(w.output, Wrap(w.graphics.DeleteImages()))
		return err
	}
	return nil
}
