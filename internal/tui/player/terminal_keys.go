package player

import (
	"fmt"
	"io"
	"os"
	"strconv"
	"strings"
	"unicode/utf8"

	"github.com/muesli/cancelreader"
	"golang.org/x/term"
)

func readPlaybackKeys(input io.Reader) (<-chan string, func(), error) {
	f, ok := input.(*os.File)
	if !ok || !term.IsTerminal(int(f.Fd())) {
		return nil, nil, fmt.Errorf("video playback requires terminal input")
	}
	state, err := term.MakeRaw(int(f.Fd()))
	if err != nil {
		return nil, nil, err
	}
	r, err := cancelreader.NewReader(f)
	if err != nil {
		_ = term.Restore(int(f.Fd()), state)
		return nil, nil, err
	}
	keys := make(chan string, 16)
	stop, done := make(chan struct{}), make(chan struct{})
	go func() {
		defer close(done)
		defer close(keys)
		buf := make([]byte, 256)
		for {
			n, err := r.Read(buf)
			if n > 0 {
				select {
				case keys <- string(buf[:n]):
				case <-stop:
					return
				}
			}
			if err != nil {
				return
			}
		}
	}()
	return keys, func() {
		close(stop)
		r.Cancel()
		<-done
		_ = r.Close()
		_ = term.Restore(int(f.Fd()), state)
	}, nil
}

var escapeKeys = map[string]string{
	"\x1b[A": "UP", "\x1b[B": "DOWN", "\x1b[C": "RIGHT", "\x1b[D": "LEFT",
	"\x1bOA": "UP", "\x1bOB": "DOWN", "\x1bOC": "RIGHT", "\x1bOD": "LEFT",
	"\x1b[H": "HOME", "\x1b[F": "END", "\x1b[5~": "PGUP", "\x1b[6~": "PGDWN",
	"\x1b[3~": "DEL", "\x1b[2~": "INS",
	"\x1b[Z": "Shift+TAB",
}

type playbackInput struct {
	Key                   string
	Mouse                 *playbackMouse
	CellWidth, CellHeight float64
}

type playbackMouse struct {
	Button, X, Y int // SGR button flags and zero-based cell coordinates.
	Pressed      bool
}

// playbackKeys preserves incomplete escape/UTF-8 sequences between pipe reads.
func playbackInputs(input string, flush bool) (inputs []playbackInput, pending string) {
	key := func(s string) { inputs = append(inputs, playbackInput{Key: s}) }
	for input != "" {
		if strings.HasPrefix(input, "\x1b[<") {
			end := strings.IndexAny(input[3:], "Mm")
			if end < 0 {
				if !flush && len(input) <= 64 {
					return inputs, input
				}
				return inputs, "" // Drop malformed reports without sending their digits to mpv.
			}
			end += 3
			if mouse := parsePlaybackMouse(input[3:end], input[end] == 'M'); mouse != nil {
				inputs = append(inputs, playbackInput{Mouse: mouse})
			}
			input = input[end+1:]
			continue
		}
		matched, partial := false, false
		if input[0] == '\x1b' {
			for sequence, name := range escapeKeys {
				if strings.HasPrefix(input, sequence) {
					key(name)
					input = input[len(sequence):]
					matched = true
					break
				}
				partial = partial || strings.HasPrefix(sequence, input)
			}
			if matched {
				continue
			}
			if partial && !flush {
				return inputs, input
			}
			// Ignore other CSI reports (including pixel/cursor replies), rather
			// than treating their numbers as mpv volume or picture shortcuts.
			if strings.HasPrefix(input, "\x1b[") {
				end := 2
				for end < len(input) && (input[end] < 0x40 || input[end] > 0x7e) {
					end++
				}
				if end == len(input) {
					if !flush && len(input) < 64 {
						return inputs, input
					}
					return inputs, ""
				}
				if input[end] == 't' {
					var h, w int
					if n, _ := fmt.Sscanf(input[2:end], "6;%d;%d", &h, &w); n == 2 && h > 0 && w > 0 && h <= 1024 && w <= 1024 {
						inputs = append(inputs, playbackInput{CellWidth: float64(w), CellHeight: float64(h)})
					}
				}
				input = input[end+1:]
				continue
			}
		}
		if !utf8.FullRuneInString(input) && !flush {
			return inputs, input
		}
		r, size := utf8.DecodeRuneInString(input)
		input = input[size:]
		switch r {
		case ' ':
			key("SPACE")
		case '\r', '\n':
			key("ENTER")
		case '\t':
			key("TAB")
		case '\x1b':
			key("ESC")
		case '\x03':
			key("q")
		case '\x7f', '\b':
			key("BS")
		default:
			if r >= 32 && r != utf8.RuneError {
				key(string(r))
			}
		}
	}
	return inputs, ""
}

func parsePlaybackMouse(report string, pressed bool) *playbackMouse {
	fields := strings.Split(report, ";")
	if len(fields) != 3 {
		return nil
	}
	button, err := strconv.Atoi(fields[0])
	x, xErr := strconv.Atoi(fields[1])
	y, yErr := strconv.Atoi(fields[2])
	if err != nil || xErr != nil || yErr != nil || x <= 0 || y <= 0 || button < 0 || button >= 128 {
		return nil
	}
	return &playbackMouse{Button: button, X: x - 1, Y: y - 1, Pressed: pressed}
}

func (m playbackMouse) key() string {
	if !m.Pressed || m.Button&32 != 0 {
		return ""
	}
	button := m.Button
	if button&64 != 0 {
		switch button & 3 {
		case 0, 3:
			return "RIGHT"
		case 1, 2:
			return "LEFT"
		}
	}
	switch button & 3 {
	case 0:
		return "SPACE"
	case 2:
		return "q"
	}
	return ""
}
