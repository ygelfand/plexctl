//go:build integration && !windows

package player

import (
	"bytes"
	"encoding/binary"
	"math"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/ygelfand/plexctl/internal/config"
)

func testAudio(t *testing.T, dir string) string {
	t.Helper()
	const rate, frames = 48000, 48000 * 6
	var wav bytes.Buffer
	wav.WriteString("RIFF")
	_ = binary.Write(&wav, binary.LittleEndian, uint32(36+frames*4))
	wav.WriteString("WAVEfmt ")
	for _, value := range []any{uint32(16), uint16(1), uint16(2), uint32(rate), uint32(rate * 4), uint16(4), uint16(16)} {
		_ = binary.Write(&wav, binary.LittleEndian, value)
	}
	wav.WriteString("data")
	_ = binary.Write(&wav, binary.LittleEndian, uint32(frames*4))
	for i := range frames {
		frequency := 200.0
		if i >= rate*2 {
			frequency = 4000
		}
		value := int16(14000 * math.Sin(2*math.Pi*frequency*float64(i)/rate))
		if i >= rate*4 {
			value = 0
		}
		_ = binary.Write(&wav, binary.LittleEndian, value)
		_ = binary.Write(&wav, binary.LittleEndian, value/2)
	}
	path := filepath.Join(dir, "tones.wav")
	if err := os.WriteFile(path, wav.Bytes(), 0o600); err != nil {
		t.Fatal(err)
	}
	return path
}

func waitAudio(t *testing.T, pm *PlayerManager, label string, condition func(PlayerStatus) bool) PlayerStatus {
	t.Helper()
	deadline := time.Now().Add(5 * time.Second)
	for time.Now().Before(deadline) {
		status := pm.Status()
		if condition(status) {
			return status
		}
		time.Sleep(10 * time.Millisecond)
	}
	t.Fatalf("%s: status %+v", label, pm.Status())
	return PlayerStatus{}
}

func TestAudioPlaybackAndSpectrum(t *testing.T) {
	mpv, err := exec.LookPath("mpv")
	if err != nil {
		t.Skip("mpv is not installed")
	}
	dir, err := os.MkdirTemp("", "plexctl-audio-")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if err := os.RemoveAll(dir); err != nil {
			t.Error(err)
		}
	})
	media := testAudio(t, dir)
	socket := filepath.Join(dir, "ipc")
	cmd := exec.Command(mpv, "--no-config", "--ao=null", "--idle=yes", "--vid=no",
		"--force-window=no", "--input-ipc-server="+socket)
	if err := cmd.Start(); err != nil {
		t.Fatal(err)
	}
	defer func() { _ = cmd.Process.Kill(); _ = cmd.Wait() }()
	deadline := time.Now().Add(5 * time.Second)
	for time.Now().Before(deadline) {
		if _, err := os.Stat(socket); err == nil {
			break
		}
		time.Sleep(10 * time.Millisecond)
	}
	pm := newPlayerManager(socket)
	defer pm.Close()
	defer pm.StopPlayback()()
	if msg := pm.Play(media, "Audio QA", "qa", true, config.AudioTerminal, 0, nil)(); msg != nil {
		t.Fatalf("play: %v", msg)
	}
	status := waitAudio(t, pm, "audio and waveform start", func(s PlayerStatus) bool {
		return s.Time > 0.2 && s.Spectrum.Valid && s.Waveform.Valid
	})
	if !status.AudioOnly || status.Spectrum.Left <= status.Spectrum.Right+5 {
		t.Fatal("stereo levels did not preserve the source's 6 dB channel difference")
	}
	wavePeak := func(samples [WaveformSamples]float64) float64 {
		peak := 0.0
		for _, value := range samples {
			peak = max(peak, math.Abs(value))
		}
		return peak
	}
	left, right := wavePeak(status.Waveform.Left), wavePeak(status.Waveform.Right)
	if left < .1 || right < .05 || left < right*1.2 {
		t.Fatalf("waveform did not reflect stereo amplitudes: left %.3f, right %.3f", left, right)
	}
	if status.Waveform.Left == status.Waveform.Right {
		t.Fatal("independent stereo waveforms were lost")
	}
	peak := func(s PlayerStatus) int {
		band := 0
		for i, value := range s.Spectrum.Bands {
			if value > s.Spectrum.Bands[band] {
				band = i
			}
		}
		return band
	}
	if band := peak(status); band != 3 && band != 4 {
		t.Fatalf("200 Hz tone appeared in band %d", band)
	}
	pm.ioMu.Lock()
	var window, video bool
	var vo string
	_ = pm.conn.get("force-window", &window)
	_ = pm.conn.get("vo-configured", &video)
	_ = pm.conn.get("current-vo", &vo)
	pm.ioMu.Unlock()
	if window || video && vo != "null" {
		t.Fatal("terminal music enabled a video window")
	}
	pm.TogglePause()()
	paused := waitAudio(t, pm, "pause", func(s PlayerStatus) bool { return s.Paused() })
	time.Sleep(150 * time.Millisecond)
	if math.Abs(pm.Status().Time-paused.Time) > 0.01 || pm.Status().Waveform.Serial != paused.Waveform.Serial {
		t.Fatal("paused playback kept advancing")
	}
	pm.Volume(-10)()
	waitAudio(t, pm, "volume", func(s PlayerStatus) bool { return s.Volume == 90 })
	pm.ToggleMute()()
	waitAudio(t, pm, "mute", func(s PlayerStatus) bool { return s.Muted })
	pm.SeekPercent(50)()
	pm.TogglePause()()
	waitAudio(t, pm, "seek updates frequencies", func(s PlayerStatus) bool {
		return s.Time >= 3 && s.Spectrum.Valid && peak(s) == 10 &&
			s.Waveform.Valid && s.Waveform.Serial > status.Waveform.Serial &&
			s.Waveform.Left != status.Waveform.Left
	})
	pm.SeekPercent(85)()
	waitAudio(t, pm, "silence", func(s PlayerStatus) bool {
		return s.Time >= 5 && s.Spectrum.Valid && s.Spectrum.Left < -90 &&
			s.Waveform.Valid && wavePeak(s.Waveform.Left) < .04 && wavePeak(s.Waveform.Right) < .04
	})
	// Reload on the same mpv connection: per-file graphs and script hooks must
	// reset, even after seeking into silence on the previous track.
	if msg := pm.Play(media, "Next track", "qa-next", true, config.AudioTerminal, 0, nil)(); msg != nil {
		t.Fatalf("next track: %v", msg)
	}
	waitAudio(t, pm, "next track waveform", func(s PlayerStatus) bool {
		return s.Key == "qa-next" && s.Time > .2 && s.Time < 2 &&
			s.Waveform.Valid && wavePeak(s.Waveform.Left) > .1
	})
	pm.StopPlayback()()
	stopped := pm.Status()
	if stopped.Active() || stopped.Spectrum.Valid || stopped.Waveform.Valid {
		t.Fatal("stop left audio or visualization active")
	}
	scriptDir := pm.waveformDir
	pm.Close()
	if _, err := os.Stat(scriptDir); !os.IsNotExist(err) {
		t.Fatal("waveform script directory was not cleaned up")
	}
}

func TestSpectrumFilterPreservesStereoSamples(t *testing.T) {
	ffmpeg, err := exec.LookPath("ffmpeg")
	if err != nil {
		t.Skip("ffmpeg is not installed")
	}
	media := testAudio(t, t.TempDir())
	graph := strings.TrimSuffix(strings.TrimPrefix(spectrumFilter(), "@plexctl-spectrum:lavfi=["), "]")
	decode := func(filter string) []byte {
		args := []string{"-v", "error", "-i", media}
		if filter != "" {
			args = append(args, "-af", filter)
		}
		args = append(args, "-f", "s16le", "pipe:1")
		cmd := exec.Command(ffmpeg, args...)
		var errors bytes.Buffer
		cmd.Stderr = &errors
		output, err := cmd.Output()
		if err != nil {
			t.Fatalf("decode: %v: %s", err, errors.String())
		}
		return output
	}
	if !bytes.Equal(decode(""), decode(graph)) {
		t.Fatal("spectrum analysis changed the audible stereo samples")
	}
}
