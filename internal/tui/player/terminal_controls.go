package player

import (
	"fmt"
	"math"
	"strings"
	"unicode"

	"github.com/ygelfand/plexctl/internal/termgraphics"
)

// The controls are composited by mpv into the Kitty image. Drawing terminal
// text over that image would be erased on every video frame. A single layout
// supplies both the ASS overlay and the mouse hit targets.
type terminalControls struct {
	hidden        bool
	focus         int
	hover         int
	dragging      bool
	speed         float64
	subtitles     bool
	canFullscreen bool
	fullscreen    bool
	lastOverlay   string
}

type videoRect struct{ x, y, w, h float64 }

func (r videoRect) contains(x, y float64) bool {
	return x >= r.x && x < r.x+r.w && y >= r.y && y < r.y+r.h
}

type videoButton struct {
	videoRect
	label   string
	command []any
}

type videoControlsLayout struct {
	w, h      int
	bar, seek videoRect
	buttons   []videoButton
}

func (c *terminalControls) layout(frame termgraphics.Frame, status PlayerStatus) videoControlsLayout {
	if frame.Width <= 0 || frame.Height <= 0 {
		return videoControlsLayout{}
	}
	w := max(720, min(1080, frame.Width))
	if c.canFullscreen {
		w = max(w, 840)
	}
	h := max(120, int(float64(w)*float64(frame.Height)/float64(frame.Width)))
	l := videoControlsLayout{w: w, h: h, bar: videoRect{0, float64(h) - 112, float64(w), 112},
		seek: videoRect{18, float64(h) - 71, float64(w) - 36, 20}}
	pause := "Pause"
	if status.Paused() {
		pause = "Play"
	}
	volume := fmt.Sprintf("Vol %.0f%%", status.Volume)
	if status.Muted {
		volume = "Muted"
	}
	subs := "CC off"
	if c.subtitles {
		subs = "CC on"
	}
	add := func(x, width float64, label string, command ...any) {
		l.buttons = append(l.buttons, videoButton{videoRect{x, float64(h) - 43, width, 32}, label, command})
	}
	add(18, 80, pause, "cycle", "pause")
	add(106, 58, "-10s", "seek", -10, "relative+exact")
	add(172, 58, "+10s", "seek", 10, "relative+exact")
	add(238, 100, volume, "cycle", "mute")
	add(346, 36, "-", "add", "volume", -5)
	add(390, 36, "+", "add", "volume", 5)
	add(434, 72, subs, "cycle", "sub-visibility")
	nextSpeed := c.speed + .25
	if nextSpeed > 2 {
		nextSpeed = .5
	}
	add(514, 62, fmt.Sprintf("%gx", c.speed), "set_property", "speed", nextSpeed)
	if c.canFullscreen {
		label := "Fullscreen"
		if c.fullscreen {
			label = "Restore"
		}
		add(float64(w)-238, 104, label, "cycle", "fullscreen")
	}
	add(float64(w)-126, 108, "Return", "quit")
	return l
}

func videoTime(seconds float64) string {
	if math.IsNaN(seconds) || math.IsInf(seconds, 0) || seconds < 0 {
		seconds = 0
	}
	s := int(seconds)
	if s >= 3600 {
		return fmt.Sprintf("%d:%02d:%02d", s/3600, s/60%60, s%60)
	}
	return fmt.Sprintf("%d:%02d", s/60, s%60)
}

// Media titles are untrusted display text, never ASS tags or escape sequences.
func videoLabel(text string, limit int) string {
	runes := []rune(strings.Map(func(r rune) rune {
		switch r {
		case '{':
			return '｛'
		case '}':
			return '｝'
		case '\\':
			return '＼'
		}
		if unicode.IsControl(r) {
			return ' '
		}
		return r
	}, text))
	if len(runes) > limit {
		return string(runes[:max(0, limit-1)]) + "…"
	}
	return string(runes)
}

func (c *terminalControls) overlay(l videoControlsLayout, status PlayerStatus) string {
	if c.hidden || l.w == 0 {
		return ""
	}
	var b strings.Builder
	rect := func(r videoRect, color string, alpha int) {
		fmt.Fprintf(&b, "{\\an7\\pos(0,0)\\bord0\\shad0\\1c&H%s&\\1a&H%02X&\\p1}m %.1f %.1f l %.1f %.1f %.1f %.1f %.1f %.1f{\\p0}\n",
			color, alpha, r.x, r.y, r.x+r.w, r.y, r.x+r.w, r.y+r.h, r.x, r.y+r.h)
	}
	text := func(x, y float64, size, anchor int, color, label string) {
		fmt.Fprintf(&b, "{\\an%d\\pos(%.1f,%.1f)\\fnSans\\fs%d\\bord0\\shad0\\b0\\q2\\1a&H00&\\1c&H%s&}%s\n", anchor, x, y, size, color, label)
	}
	rect(l.bar, "211B15", 24)
	title := videoLabel(status.Title, (l.w-420)/11) + "   [h: hide]"
	if c.canFullscreen {
		title = videoLabel(status.Title, max(0, (l.w-580)/11)) + "   [f: fullscreen, h: hide]"
	}
	if c.focus >= 0 {
		title = "Tab: next control   Enter: activate   Esc: return"
	}
	text(18, l.bar.y+9, 19, 7, "F4F1ED", title)
	text(float64(l.w)-18, l.bar.y+9, 19, 9, "D3B94D", videoTime(status.Time)+" / "+videoTime(status.Duration))
	track := videoRect{l.seek.x, l.seek.y + 7, l.seek.w, 6}
	rect(track, "645951", 0)
	if status.Duration > 0 {
		fraction := max(0, min(1, status.Time/status.Duration))
		fill := track
		fill.w *= fraction
		rect(fill, "E1CA58", 0)
		rect(videoRect{track.x + fill.w - 3, track.y - 4, 6, 14}, "F4F1ED", 0)
	}
	for i, button := range l.buttons {
		color := "46392D"
		if i == 0 {
			color = "856725"
		}
		if i == c.hover || i == c.focus {
			color = "B39539"
		}
		rect(button.videoRect, color, 0)
		text(button.x+button.w/2, button.y+button.h/2, 18, 5, "FFFFFF", button.label)
	}
	return b.String()
}

// position uses the actual graphics placement, not the movie's source size or
// an assumed full-terminal image. SGR mouse coordinates identify cell centers.
func videoMousePosition(m playbackMouse, frame termgraphics.Frame, cellW, cellH float64, l videoControlsLayout) (float64, float64) {
	if frame.Width <= 0 || frame.Height <= 0 || cellW <= 0 || cellH <= 0 {
		return -1, -1
	}
	x := (float64(m.X-frame.X) + .5) * cellW * float64(l.w) / float64(frame.Width)
	y := (float64(m.Y-frame.Y) + .5) * cellH * float64(l.h) / float64(frame.Height)
	return x, y
}

func (c *terminalControls) input(input playbackInput, l videoControlsLayout, frame termgraphics.Frame, cellW, cellH float64, status PlayerStatus) []any {
	if input.Mouse == nil {
		switch input.Key {
		case "f":
			if c.canFullscreen {
				return []any{"cycle", "fullscreen"}
			}
		case "h":
			c.hidden, c.dragging, c.focus = !c.hidden, false, -1
			return nil
		case "TAB", "Shift+TAB":
			c.hidden = false
			if len(l.buttons) > 0 {
				delta := 1
				if input.Key == "Shift+TAB" {
					delta = -1
					if c.focus < 0 {
						c.focus = 0
					}
				}
				c.focus = (c.focus + delta + len(l.buttons)) % len(l.buttons)
			}
			return nil
		case "ENTER":
			if !c.hidden && c.focus >= 0 && c.focus < len(l.buttons) {
				return l.buttons[c.focus].command
			}
			return []any{"cycle", "pause"}
		case "ESC":
			return []any{"quit"}
		case "+", "=":
			return []any{"add", "volume", 5}
		case "-":
			return []any{"add", "volume", -5}
		}
		return []any{"keypress", input.Key}
	}
	m := *input.Mouse
	x, y := videoMousePosition(m, frame, cellW, cellH, l)
	motion, wheel := m.Button&32 != 0, m.Button&64 != 0
	if m.Pressed && !motion && !wheel && m.Button&3 == 2 {
		return []any{"quit"}
	}
	c.hover = -1
	if !c.hidden {
		for i, b := range l.buttons {
			if b.contains(x, y) {
				c.hover = i
				break
			}
		}
	}
	if wheel && m.Pressed {
		delta := 5
		if m.Button&3 == 1 || m.Button&3 == 2 {
			delta = -5
		}
		if c.hover >= 3 && c.hover <= 5 {
			return []any{"add", "volume", delta}
		}
		return []any{"seek", delta * 2, "relative+exact"}
	}
	seek := func() []any {
		if status.Duration <= 0 || l.seek.w <= 0 {
			return nil
		}
		seconds := max(0, min(1, (x-l.seek.x)/l.seek.w)) * status.Duration
		return []any{"seek", seconds, "absolute+exact"}
	}
	if !m.Pressed {
		wasDragging := c.dragging
		c.dragging = false
		if wasDragging {
			return seek()
		}
		return nil
	}
	if motion {
		if c.dragging && m.Button&3 == 0 {
			return seek()
		}
		return nil
	}
	if m.Button&3 != 0 {
		return nil
	}
	c.focus = -1
	if !c.hidden && l.seek.contains(x, y) {
		c.dragging = status.Duration > 0
		return seek()
	}
	if c.hover >= 0 {
		return l.buttons[c.hover].command
	}
	if !c.hidden && l.bar.contains(x, y) {
		return nil
	}
	return []any{"cycle", "pause"}
}
