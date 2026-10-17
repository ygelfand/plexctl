package player

import (
	"reflect"
	"slices"
	"testing"
)

// Keep the legacy bindings covered for renderers without a graphical bar.
func playbackKeys(input string, flush bool) (keys []string, pending string) {
	inputs, pending := playbackInputs(input, flush)
	for _, input := range inputs {
		key := input.Key
		if input.Mouse != nil {
			key = input.Mouse.key()
		}
		if key != "" {
			keys = append(keys, key)
		}
	}
	return keys, pending
}

func TestPlaybackMouseCoordinatesAndMetrics(t *testing.T) {
	for _, tc := range []struct {
		input string
		want  playbackInput
	}{
		{"\x1b[<0;20;8M", playbackInput{Mouse: &playbackMouse{Button: 0, X: 19, Y: 7, Pressed: true}}},
		{"\x1b[<32;20;8M", playbackInput{Mouse: &playbackMouse{Button: 32, X: 19, Y: 7, Pressed: true}}},
		{"\x1b[<0;20;8m", playbackInput{Mouse: &playbackMouse{Button: 0, X: 19, Y: 7, Pressed: false}}},
		{"\x1b[6;18;9t", playbackInput{CellWidth: 9, CellHeight: 18}},
	} {
		for split := 1; split < len(tc.input); split++ {
			first, pending := playbackInputs(tc.input[:split], false)
			last, pending := playbackInputs(pending+tc.input[split:], false)
			if got := append(first, last...); pending != "" || !reflect.DeepEqual(got, []playbackInput{tc.want}) {
				t.Fatalf("report %q split %d: got %+v, pending %q", tc.input, split, got, pending)
			}
		}
	}
	for _, input := range []string{"\x1b[4;900;1500t", "\x1b[?1;2c", "\x1b[6;0;8t", "\x1b[<0;bad;8M"} {
		if got, _ := playbackInputs(input, true); len(got) != 0 {
			t.Fatalf("terminal report leaked as input: %q: %+v", input, got)
		}
	}
}

func TestPlaybackKeys(t *testing.T) {
	keys, pending := playbackKeys(" \x1b[Cq\x1b[", false)
	if !slices.Equal(keys, []string{"SPACE", "RIGHT", "q"}) || pending != "\x1b[" {
		t.Fatalf("keys=%v pending=%q", keys, pending)
	}
	keys, pending = playbackKeys(pending+"D\x03", false)
	if !slices.Equal(keys, []string{"LEFT", "q"}) || pending != "" {
		t.Fatalf("split arrow: %v, %q", keys, pending)
	}
	keys, pending = playbackKeys("\x1b", true)
	if !slices.Equal(keys, []string{"ESC"}) || pending != "" {
		t.Fatalf("escape timeout: %v, %q", keys, pending)
	}
}

func TestPlaybackMouseReports(t *testing.T) {
	for _, tc := range []struct{ input, want string }{
		{"\x1b[<0;20;8M", "SPACE"},
		{"\x1b[<0;20;8m", ""},
		{"\x1b[<32;20;8M", ""},
		{"\x1b[<64;20;8M", "RIGHT"},
		{"\x1b[<65;20;8M", "LEFT"},
		{"\x1b[<2;20;8M", "q"},
		{"\x1b[<0;bad;8M", ""},
	} {
		for split := 1; split < len(tc.input); split++ {
			first, pending := playbackKeys(tc.input[:split], false)
			last, pending := playbackKeys(pending+tc.input[split:], false)
			keys := append(first, last...)
			if pending != "" || (tc.want == "" && len(keys) != 0) || (tc.want != "" && !slices.Equal(keys, []string{tc.want})) {
				t.Fatalf("report %q split %d: keys=%v pending=%q", tc.input, split, keys, pending)
			}
		}
	}
}
