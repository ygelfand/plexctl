// Package visualizer renders audio levels with ordinary terminal cells. It
// requires neither terminal graphics extensions nor access to a sound device.
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

type Model struct {
	levels    [player.SpectrumBands]float64
	peaks     [player.SpectrumBands]float64
	hold      [player.SpectrumBands]float64
	left      float64
	right     float64
	valid     bool
	waves     [3]player.Waveform
	waveFade  float64
	waveReady bool
}

func amplitude(db float64) float64 {
	if math.IsNaN(db) || math.IsInf(db, 0) {
		return 0
	}
	return math.Pow(max(0, min(1, (db+65)/57)), 1.25)
}

// Step uses quick attack, slower release, and independent peak hold. Silence
// and pause settle to zero; every movement is driven by measured audio levels.
func (m *Model) Step(frame player.Spectrum, paused bool, elapsed time.Duration) {
	dt := max(0, min(0.25, elapsed.Seconds()))
	active := frame.Valid && !paused
	m.valid = m.valid || frame.Valid
	for i := range m.levels {
		target := 0.0
		if active {
			target = amplitude(frame.Bands[i])
		}
		tau := 0.16
		if target > m.levels[i] {
			tau = 0.035
		}
		m.levels[i] += (target - m.levels[i]) * (1 - math.Exp(-dt/tau))
		if m.levels[i] >= m.peaks[i] {
			m.peaks[i], m.hold[i] = m.levels[i], 0.35
		} else if m.hold[i] > 0 {
			m.hold[i] -= dt
		} else {
			m.peaks[i] = max(m.levels[i], m.peaks[i]-dt*0.42)
		}
	}
	left, right := 0.0, 0.0
	if active {
		left, right = amplitude(frame.Left), amplitude(frame.Right)
	}
	m.left += (left - m.left) * (1 - math.Exp(-dt/0.08))
	m.right += (right - m.right) * (1 - math.Exp(-dt/0.08))
}

var palette = [...]string{
	"#22d3a6", "#22d3bd", "#22d3d3", "#26c5ec", "#38b5ff", "#559cff", "#7585ff", "#9470ff",
	"#b36bff", "#cf68f5", "#e868df", "#f768c1", "#fb769e", "#ff897d", "#ffa665", "#ffc857",
}

// View returns height rows of spectrum plus one row of stereo meters.
func (m *Model) View(width, height int, mode string) string {
	if width < 1 || height < 1 {
		return ""
	}
	if mode == "waveform" {
		return m.waveformView(width, height)
	}
	count := max(1, min(player.SpectrumBands, width/3))
	step := max(1, width/count)
	barWidth := max(1, step-1)
	rows := make([]string, height)
	muted := lipgloss.NewStyle().Foreground(lipgloss.Color("#334155"))
	peak := lipgloss.NewStyle().Foreground(lipgloss.Color("#e5f4ff"))
	for row := range height {
		var line strings.Builder
		for band := range count {
			start, end := band*player.SpectrumBands/count, (band+1)*player.SpectrumBands/count
			level, held := 0.0, 0.0
			for i := start; i < end; i++ {
				level, held = max(level, m.levels[i]), max(held, m.peaks[i])
			}
			units := level*float64(height) - float64(height-1-row)
			if mode == "mirror" {
				center := float64(height-1) / 2
				units = level*float64(height)/2 - math.Abs(float64(row)-center) + 0.5
			}
			glyph := block(units)
			style := lipgloss.NewStyle().Foreground(lipgloss.Color(palette[start]))
			if mode == "spectrum" && held > 0.025 && row == height-1-int(held*float64(height-1)) && units < 0.9 {
				glyph, style = "▔", peak
			}
			if glyph == " " && row == height-1 {
				glyph, style = "·", muted
			}
			line.WriteString(style.Render(strings.Repeat(glyph, barWidth)))
			line.WriteString(strings.Repeat(" ", step-barWidth))
		}
		rows[row] = lipgloss.NewStyle().Width(width).MaxWidth(width).Render(line.String())
	}
	label := "40 Hz · SPECTRUM · 16 kHz"
	if mode == "mirror" {
		label = "40 Hz · MIRROR · 16 kHz"
	}
	if !m.valid {
		label = "Waiting for audio levels"
	}
	meterWidth := max(1, min(10, (width-lipgloss.Width(label)-9)/2))
	left := meter("L", m.left, meterWidth, palette[3])
	right := meter("R", m.right, meterWidth, palette[11])
	meters := left + " " + right
	gap := max(1, width-lipgloss.Width(meters)-lipgloss.Width(label))
	legend := meters + strings.Repeat(" ", gap) + muted.Render(label)
	rows = append(rows, ui.Ellipsis(legend, width))
	return strings.Join(rows, "\n")
}

func block(value float64) string {
	const blocks = " ▁▂▃▄▅▆▇█"
	return string([]rune(blocks)[int(math.Round(max(0, min(1, value))*8))])
}

func meter(label string, value float64, width int, color string) string {
	filled := int(math.Round(max(0, min(1, value)) * float64(width)))
	return fmt.Sprintf("%s %s%s", label,
		lipgloss.NewStyle().Foreground(lipgloss.Color(color)).Render(strings.Repeat("━", filled)),
		lipgloss.NewStyle().Foreground(lipgloss.Color("#334155")).Render(strings.Repeat("─", width-filled)))
}
