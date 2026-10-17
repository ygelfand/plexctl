package termgraphics

import (
	"bufio"
	"bytes"
	"fmt"
	"io"
	"strconv"
	"strings"
	"sync"

	"github.com/charmbracelet/x/ansi"
)

// Writer wraps mpv's graphics packets for tmux, retaining ANSI cursor movement
// and screen switching in tmux itself so playback stays inside its pane.
// Packets can span arbitrary writes from os/exec's stdout pipe.
type Writer struct {
	Output io.Writer
	// Direct observes graphics without wrapping them for tmux.
	Direct bool
	// ImageID reserves this ID and the next for alternating video frames.
	// Replacing an image in place deletes it before the upload finishes.
	ImageID          int
	displayedImage   int
	uploadingImage   int
	buffered         *bufio.Writer
	pending          []byte
	cursorX, cursorY int
	mu               sync.RWMutex
	frame            Frame
	originX, originY int
	originSet        bool
}

// SetPaneOrigin anchors each passthrough packet in the physical terminal.
// tmux forwards raw graphics without moving its cursor and resets that cursor
// between packets; a pane-relative CSI alone cannot position a split's video.
// It returns whether the placement changed, so paused video can be redrawn.
func (w *Writer) SetPaneOrigin(x, y int) bool {
	w.mu.Lock()
	defer w.mu.Unlock()
	changed := !w.originSet || w.originX != x || w.originY != y
	w.originX, w.originY, w.originSet = x, y, true
	return changed
}

// Frame describes the actual Kitty placement: a cell origin and pixel size.
// Read it while playback is running to map terminal mouse reports to the image,
// including letterboxing, custom video geometry, and terminal resizing.
type Frame struct {
	X, Y, Width, Height int
}

func (w *Writer) Frame() Frame {
	w.mu.RLock()
	defer w.mu.RUnlock()
	return w.frame
}

func (w *Writer) Write(p []byte) (n int, err error) {
	if w.buffered == nil {
		w.buffered = bufio.NewWriterSize(w.Output, 32<<10)
	}
	// A video frame contains hundreds of 4 KiB packets. Batch their writes,
	// particularly through tmux, but flush every input chunk so paused frames
	// and the final cleanup never wait for more output from mpv.
	defer func() {
		if flushErr := w.buffered.Flush(); err == nil && flushErr != nil {
			n, err = 0, flushErr
		}
	}()
	w.pending = append(w.pending, p...)
	for len(w.pending) > 0 {
		start := bytes.IndexByte(w.pending, '\x1b')
		if start < 0 {
			start = len(w.pending)
		}
		if start > 0 {
			if _, err := w.buffered.Write(w.pending[:start]); err != nil {
				return 0, err
			}
			w.pending = w.pending[start:]
			continue
		}
		if len(w.pending) < 2 {
			break
		}
		if w.pending[1] == '[' {
			end := 2
			for end < len(w.pending) && (w.pending[end] < 0x40 || w.pending[end] > 0x7e) {
				end++
			}
			if end == len(w.pending) {
				if end > 4096 {
					return 0, fmt.Errorf("terminal cursor sequence exceeds 4 KiB")
				}
				break
			}
			if w.pending[end] == 'H' || w.pending[end] == 'f' {
				row, col, _ := strings.Cut(string(w.pending[2:end]), ";")
				y, _ := strconv.Atoi(row)
				x, _ := strconv.Atoi(col)
				w.cursorX, w.cursorY = max(0, x-1), max(0, y-1)
			}
			if _, err := w.buffered.Write(w.pending[:end+1]); err != nil {
				return 0, err
			}
			w.pending = w.pending[end+1:]
			continue
		}
		if w.pending[1] != '_' && w.pending[1] != 'P' {
			if _, err := w.buffered.Write(w.pending[:2]); err != nil {
				return 0, err
			}
			w.pending = w.pending[2:]
			continue
		}
		end := bytes.Index(w.pending[2:], []byte("\x1b\\"))
		if end < 0 {
			if len(w.pending) > 32<<20 {
				return 0, fmt.Errorf("terminal graphics packet exceeds 32 MiB")
			}
			break
		}
		end += 4
		sequence := string(w.pending[:end])
		if strings.HasPrefix(sequence, "\x1b_Ga=T,") {
			head, _, _ := strings.Cut(sequence[3:], ";")
			frame := Frame{X: w.cursorX, Y: w.cursorY}
			for field := range strings.SplitSeq(head, ",") {
				key, value, _ := strings.Cut(field, "=")
				switch key {
				case "s":
					frame.Width, _ = strconv.Atoi(value)
				case "v":
					frame.Height, _ = strconv.Atoi(value)
				}
			}
			w.mu.Lock()
			w.frame = frame
			w.mu.Unlock()
		}
		if w.ImageID != 0 && strings.HasPrefix(sequence, "\x1b_G") {
			sequence = w.kittyFrame(sequence)
		}
		packet := sequence
		if !w.Direct {
			w.mu.RLock()
			if w.originSet {
				// Keep the absolute cursor move inside the same passthrough as
				// the graphics, including continuation packets after a reset.
				sequence = fmt.Sprintf("\x1b[%d;%dH", w.originY+w.cursorY+1, w.originX+w.cursorX+1) + sequence
			}
			w.mu.RUnlock()
			packet = ansi.TmuxPassthrough(sequence)
		}
		if _, err := w.buffered.WriteString(packet); err != nil {
			return 0, err
		}
		w.pending = w.pending[end:]
	}
	return len(p), nil
}

func (w *Writer) kittyFrame(sequence string) string {
	head, payload, ok := strings.Cut(sequence, ";")
	if !ok {
		return sequence
	}
	fields := "," + strings.TrimPrefix(head, "\x1b_G") + ","
	switch {
	case strings.Contains(head, "a=T"):
		w.uploadingImage = w.ImageID
		if w.displayedImage == w.ImageID {
			w.uploadingImage++
		}
		sequence = fmt.Sprintf("%s,i=%d;%s", head, w.uploadingImage, payload)
	case strings.Contains(head, "a=d"):
		w.displayedImage, w.uploadingImage = 0, 0
		return w.DeleteImages()
	}
	// Keep the previous frame visible until the last chunk of its replacement
	// is delivered. Reusing a single ID makes moving video flicker or go black,
	// even when mpv reports no dropped frames. Two IDs also bound image memory.
	if w.uploadingImage != 0 && (!strings.Contains(fields, ",m=1,") || strings.Contains(fields, ",t=s,")) {
		if w.displayedImage != 0 {
			sequence += DeleteImage(w.displayedImage)
		}
		w.displayedImage, w.uploadingImage = w.uploadingImage, 0
	}
	return sequence
}

// DeleteImages removes only the two images owned by this playback session.
func (w *Writer) DeleteImages() string {
	return DeleteImage(w.ImageID) + DeleteImage(w.ImageID+1)
}

func DeleteImage(id int) string {
	return fmt.Sprintf("\x1b_Ga=d,d=I,i=%d,q=2;\x1b\\", id)
}
