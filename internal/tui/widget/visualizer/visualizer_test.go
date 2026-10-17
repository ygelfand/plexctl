package visualizer

import (
	"math"
	"strings"
	"testing"
	"time"

	"github.com/charmbracelet/lipgloss"
	"github.com/ygelfand/plexctl/internal/tui/player"
)

func TestVisualizerRespondsToAudioAndSettlesOnPause(t *testing.T) {
	var m Model
	frame := player.Spectrum{Valid: true, Left: -8, Right: -20}
	for i := range frame.Bands {
		frame.Bands[i] = -100
	}
	frame.Bands[3] = -12
	for range 10 {
		m.Step(frame, false, 50*time.Millisecond)
	}
	if m.levels[3] < 0.7 || m.levels[9] != 0 || m.left <= m.right {
		t.Fatal("display did not follow the measured frequency and stereo levels")
	}
	for range 100 {
		m.Step(frame, true, 50*time.Millisecond)
	}
	if m.levels[3] > 0.001 || m.peaks[3] > 0.001 {
		t.Fatal("paused audio kept animating")
	}
}

func TestVisualizerFitsTerminalSizes(t *testing.T) {
	for _, mode := range []string{"waveform", "spectrum", "mirror"} {
		t.Run(mode, func(t *testing.T) {
			var m Model
			m.StepWaveform(testWave(), false, 50*time.Millisecond)
			for _, width := range []int{1, 12, 40, 53, 130} {
				for _, height := range []int{1, 2, 6} {
					view := m.View(width, height, mode)
					if lipgloss.Height(view) != height+1 {
						t.Fatalf("%dx%d: unexpected height %d", width, height, lipgloss.Height(view))
					}
					for line := range strings.SplitSeq(view, "\n") {
						if lipgloss.Width(line) > width {
							t.Fatalf("%dx%d: line overflows terminal", width, height)
						}
					}
				}
			}
		})
	}
}

func testWave() player.Waveform {
	frame := player.Waveform{Valid: true, Serial: 1}
	for i := range frame.Left {
		frame.Left[i] = math.Sin(float64(i) * .07)
		frame.Right[i] = math.Cos(float64(i)*.12) * .5
	}
	return frame
}

func TestWaveformFollowsSamplesAndSettles(t *testing.T) {
	var m Model
	frame := testWave()
	m.StepWaveform(frame, false, 50*time.Millisecond)
	live := m.View(80, 8, "waveform")
	if !strings.Contains(live, "LIVE WAVEFORM") || m.waves[0] != frame {
		t.Fatal("waveform did not use the decoded stereo samples")
	}
	for range 50 {
		m.StepWaveform(frame, true, 50*time.Millisecond)
	}
	if m.waveFade > .001 || m.View(80, 8, "waveform") == live {
		t.Fatal("pause did not settle the waveform")
	}
	for i := range 4 {
		m.StepWaveform(player.Waveform{Valid: true, Serial: int64(i + 2)}, false, 50*time.Millisecond)
	}
	for _, wave := range m.waves {
		if wave.Left != [player.WaveformSamples]float64{} || wave.Right != wave.Left {
			t.Fatal("silence retained an invented waveform")
		}
	}
	for range 10 {
		m.StepWaveform(player.Waveform{Valid: true, Serial: 5}, false, 50*time.Millisecond)
	}
	if m.waves[1].Serial == m.waves[0].Serial {
		t.Fatal("repeated polling incorrectly advanced waveform history")
	}
}

func TestWaveSampleRejectsInvalidValues(t *testing.T) {
	var samples [player.WaveformSamples]float64
	for _, value := range []float64{math.NaN(), math.Inf(1), math.Inf(-1)} {
		samples[0] = value
		if waveSample(&samples, 0) != 0 {
			t.Fatal("invalid amplitude escaped into rendering")
		}
	}
}
