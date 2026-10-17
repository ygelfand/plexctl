package player

import (
	"math"
	"testing"
)

func TestSpectrumHandlesSilenceAndMissingMetadata(t *testing.T) {
	for _, tt := range []struct {
		name  string
		value string
		valid bool
		level float64
	}{
		{name: "signal", value: "-20.25", valid: true, level: -20.25},
		{name: "silence", value: "-inf", valid: true, level: -100},
		{name: "invalid", value: "nan", level: -100},
		{name: "missing", level: -100},
	} {
		t.Run(tt.name, func(t *testing.T) {
			frame := readSpectrum(map[string]string{"lavfi.astats.3.RMS_level": tt.value})
			if frame.Valid != tt.valid || frame.Bands[0] != tt.level || math.IsNaN(frame.Bands[0]) {
				t.Fatalf("unexpected frame: %+v", frame)
			}
		})
	}
}
