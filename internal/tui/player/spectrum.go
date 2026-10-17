package player

import (
	"fmt"
	"math"
	"strconv"
	"strings"
)

const SpectrumBands = 16

// Spectrum is measured from the current decoded audio frame, in dBFS. It is
// independent of the sound device, graphics protocol, tmux, and SSH transport.
type Spectrum struct {
	Bands [SpectrumBands]float64
	Left  float64
	Right float64
	Valid bool
}

var bandFrequencies = [SpectrumBands]int{
	40, 63, 100, 160, 250, 400, 630, 1000,
	1600, 2500, 4000, 6300, 8000, 10000, 12500, 16000,
}

// spectrumFilter measures parallel bandpass copies, then discards those copies
// with pan. The first two channels carry the original stereo audio untouched;
// astats frame metadata survives pan and is exposed through mpv's af-metadata.
// No second decoder, microphone capture, growing file, or external tool is used.
func spectrumFilter() string {
	var graph strings.Builder
	fmt.Fprintf(&graph, "[in]aformat=sample_fmts=fltp:channel_layouts=stereo,asplit=%d[main]", SpectrumBands+1)
	for i := range SpectrumBands {
		fmt.Fprintf(&graph, "[s%d]", i)
	}
	for i, frequency := range bandFrequencies {
		fmt.Fprintf(&graph, ";[s%d]pan=mono|c0=0.5*c0+0.5*c1,bandpass=f=%d:t=o:w=0.75[b%d]", i, frequency, i)
	}
	graph.WriteString(";[main]")
	for i := range SpectrumBands {
		fmt.Fprintf(&graph, "[b%d]", i)
	}
	fmt.Fprintf(&graph, "amerge=inputs=%d,astats=metadata=1:reset=1:measure_perchannel=RMS_level:measure_overall=none,"+
		"pan=stereo|c0=c0|c1=c1[out]", SpectrumBands+1)
	return "@plexctl-spectrum:lavfi=[" + graph.String() + "]"
}

func readSpectrum(metadata map[string]string) Spectrum {
	var frame Spectrum
	level := func(channel int) float64 {
		text, ok := metadata[fmt.Sprintf("lavfi.astats.%d.RMS_level", channel)]
		value, err := strconv.ParseFloat(text, 64)
		if !ok || err != nil || math.IsNaN(value) || math.IsInf(value, 1) {
			return -100
		}
		frame.Valid = true
		return max(-100, min(0, value))
	}
	frame.Left, frame.Right = level(1), level(2)
	for i := range SpectrumBands {
		frame.Bands[i] = level(i + 3)
	}
	return frame
}
