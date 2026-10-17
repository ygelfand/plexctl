package visualizer

import (
	"fmt"
	"math"
	"strings"
	"time"

	"github.com/charmbracelet/lipgloss"
	"github.com/ygelfand/plexctl/internal/tui/player"
	"github.com/ygelfand/plexctl/internal/ui"
)

// StepWaveform retains short phosphor trails of the actual stereo trace. There
// is no time-driven oscillator: pause/silence settle instead of inventing sound.
func (m *Model) StepWaveform(frame player.Waveform, paused bool, elapsed time.Duration) {
	dt := max(0, min(.25, elapsed.Seconds()))
	if paused || !frame.Valid {
		m.waveFade *= math.Exp(-dt / .16)
		return
	}
	m.waveReady = true
	if frame.Serial != m.waves[0].Serial {
		m.waves[2], m.waves[1], m.waves[0] = m.waves[1], m.waves[0], frame
	}
	m.waveFade += (1 - m.waveFade) * (1 - math.Exp(-dt/.035))
}

func waveSample(samples *[player.WaveformSamples]float64, at float64) float64 {
	i := min(player.WaveformSamples-1, max(0, int(at)))
	next := min(player.WaveformSamples-1, i+1)
	v := samples[i] + (samples[next]-samples[i])*(at-float64(i))
	if math.IsNaN(v) || math.IsInf(v, 0) {
		return 0
	}
	return max(-1, min(1, v))
}

func waveColor(channel, position, strength int) string {
	// Cyan/azure for left; violet/rose for right. Overlapping live traces glow.
	colors := [2][3][3]int{
		{{34, 211, 166}, {56, 181, 255}, {129, 220, 255}},
		{{148, 112, 255}, {232, 104, 223}, {255, 178, 214}},
	}
	index := min(1, position/8)
	t := float64(position%8) / 8
	brightness := []float64{.18, .32, .56, 1}[min(3, strength)]
	values := [3]int{}
	for i := range values {
		values[i] = int((float64(colors[channel][index][i])*(1-t) + float64(colors[channel][index+1][i])*t) * brightness)
	}
	return fmt.Sprintf("#%02x%02x%02x", values[0], values[1], values[2])
}

func (m *Model) waveformView(width, height int) string {
	// Braille gives 2×4 addressable points per cell on Kitty, plain terminals,
	// tmux and SSH. No graphics protocol, sixel, or local audio device is needed.
	pw, ph := width*2, height*4
	pixels := make([]uint8, pw*ph)
	point := func(x, y, channel, strength int) {
		if x < 0 || x >= pw || y < 0 || y >= ph {
			return
		}
		at := y*pw + x
		value := uint8(strength | channel<<2)
		if int(pixels[at]&3) <= strength {
			pixels[at] = value
		}
	}
	separate := height >= 4
	for age := 2; age >= 0; age-- {
		frame := &m.waves[age]
		if !frame.Valid {
			continue
		}
		for channel := range 2 {
			samples := &frame.Left
			if channel == 1 {
				samples = &frame.Right
			}
			center, span := float64(ph-1)/2, float64(ph-2)/2
			if separate {
				center = float64(channel*ph/2) + float64(ph/2-1)/2
				span = float64(ph/2-2) / 2
			}
			previous := -1
			for x := range pw {
				v := waveSample(samples, float64(x)*float64(player.WaveformSamples-1)/float64(max(1, pw-1)))
				y := int(math.Round(center - v*span*m.waveFade))
				if previous < 0 {
					previous = y
				}
				for p := min(previous, y); p <= max(previous, y); p++ {
					point(x, p, channel, 3-age)
				}
				previous = y
			}
		}
	}
	styles := [2][16][4]lipgloss.Style{}
	for channel := range 2 {
		for pos := range 16 {
			for strength := range 4 {
				styles[channel][pos][strength] = lipgloss.NewStyle().Foreground(lipgloss.Color(waveColor(channel, pos, strength)))
			}
		}
	}
	bits := [4][2]rune{{1, 8}, {2, 16}, {4, 32}, {64, 128}}
	rows := make([]string, 0, height+1)
	for row := range height {
		var line strings.Builder
		for col := range width {
			var glyph rune
			strength, channel := 0, 0
			for y := range 4 {
				for x := range 2 {
					value := pixels[(row*4+y)*pw+col*2+x]
					if value&3 == 0 {
						continue
					}
					glyph |= bits[y][x]
					if int(value&3) >= strength {
						strength, channel = int(value&3), int(value>>2)
					}
				}
			}
			if glyph == 0 {
				if row == height/2 && col%4 == 0 {
					line.WriteString(styles[0][0][0].Render("·"))
				} else {
					line.WriteByte(' ')
				}
			} else {
				line.WriteString(styles[channel][min(15, col*16/width)][strength].Render(string(0x2800 + glyph)))
			}
		}
		rows = append(rows, line.String())
	}
	label := "LIVE WAVEFORM · L / R"
	if !m.waveReady {
		label = "Waiting for waveform · z: spectrum"
	}
	meters := meter("L", m.left, min(8, max(1, width/12)), palette[3]) + " " + meter("R", m.right, min(8, max(1, width/12)), palette[11])
	gap := max(1, width-lipgloss.Width(meters)-lipgloss.Width(label))
	legend := meters + strings.Repeat(" ", gap) + lipgloss.NewStyle().Foreground(lipgloss.Color("#7391ab")).Render(label)
	rows = append(rows, ui.Ellipsis(legend, width))
	return strings.Join(rows, "\n")
}
