package termgraphics

import (
	"bytes"
	"fmt"
	"image"
	"strings"
	"testing"
)

func TestOverlayConfinesTextAndOwnsNoTerminalModes(t *testing.T) {
	t.Setenv("TMUX", "")
	for _, chunk := range []int{1, 7, 4096} {
		t.Run(fmt.Sprintf("chunk-%d", chunk), func(t *testing.T) {
			var output bytes.Buffer
			w := NewOverlayWriter(&output, func() image.Rectangle { return image.Rect(28, 2, 32, 4) }, false)
			input := "\x1b[?1049h\x1b[2J\x1b[?25l\x1b[H12345\n67890\nOUTSIDE\x1b[?1049l"
			for start := 0; start < len(input); start += chunk {
				part := []byte(input[start:min(start+chunk, len(input))])
				if n, err := w.Write(part); err != nil || n != len(part) {
					t.Fatalf("chunk %d: write = %d, %v", chunk, n, err)
				}
			}
			got := output.String()
			for _, forbidden := range []string{"1049", "[2J", "?25l", "OUTSIDE"} {
				if strings.Contains(got, forbidden) {
					t.Fatalf("overlay leaked %q with chunk %d: %q", forbidden, chunk, got)
				}
			}
			if !strings.Contains(got, "\x1b[3;29H") || strings.Contains(got, "12345") || strings.Contains(got, "67890") {
				t.Fatalf("overlay coordinates or clipping incorrect: %q", got)
			}
			if !strings.Contains(got, "\x1b7") || !strings.Contains(got, "\x1b8") {
				t.Fatal("overlay did not preserve the TUI cursor")
			}
		})
	}
}

func TestOverlayRestoresBothPixelColorsAcrossWrites(t *testing.T) {
	var output bytes.Buffer
	w := NewOverlayWriter(&output, func() image.Rectangle { return image.Rect(2, 2, 6, 6) }, false)
	fg, bg := "\x1b[38;2;10;20;30m", "\x1b[48;2;40;50;60m"
	_, _ = w.Write([]byte(fg + bg))
	output.Reset()
	_, _ = w.Write([]byte("▀"))
	if !strings.Contains(output.String(), fg+bg+"▀") {
		t.Fatalf("pixel lost color after the TUI cursor was restored: %q", output.String())
	}
}

func TestOverlayBuffersGraphicsAndCleansUpOnlyItsImages(t *testing.T) {
	t.Setenv("TMUX", "")
	var output bytes.Buffer
	w := NewOverlayWriter(&output, func() image.Rectangle { return image.Rect(10, 1, 30, 10) }, true)
	packet := "\x1b_Ga=T,f=24,s=1,v=1;AAAA\x1b\\"
	for _, b := range []byte(packet) {
		if _, err := w.Write([]byte{b}); err != nil {
			t.Fatal(err)
		}
	}
	if strings.Count(output.String(), "\x1b_Ga=T") != 1 || !strings.Contains(output.String(), "AAAA\x1b\\") {
		t.Fatalf("fragmented graphics were corrupted: %q; pending %q", output.String(), w.pending)
	}
	w.SetVisible(false)
	output.Reset()
	_, _ = w.Write([]byte(packet))
	if output.Len() != 0 {
		t.Fatal("video painted over a TUI dialog")
	}
	if err := w.Close(); err != nil {
		t.Fatal(err)
	}
	if strings.Contains(output.String(), "d=A") || !strings.Contains(output.String(), "d=I") {
		t.Fatal("cleanup did not target only video-owned images")
	}
	output.Reset()
	_, _ = w.Write([]byte(packet))
	if output.Len() != 0 {
		t.Fatal("closed overlay still writes to the terminal")
	}
}
