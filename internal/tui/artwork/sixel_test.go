package artwork

import (
	"bytes"
	"image"
	"image/color"
	"io"
	"strings"
	"testing"

	"github.com/charmbracelet/lipgloss"
	"github.com/charmbracelet/x/ansi"
	"github.com/charmbracelet/x/ansi/kitty"
	"github.com/ygelfand/plexctl/internal/termgraphics"
)

func TestSixelClippingAndDuplicatePlacements(t *testing.T) {
	view := placeholders(7, 4, 4)
	lines := strings.Split(view, "\n")
	// Clip off the top and left of one image. Put a second copy to its right,
	// with an ordinary text cell replacing part of the second row.
	first := ansi.Cut(lines[1], 1, 4) + " gap " + lines[0]
	second := ansi.Cut(lines[2], 1, 4) + " gap " + ansi.Cut(lines[1], 0, 2) + "X" + ansi.Cut(lines[1], 3, 4)
	placements := scanImages(first+"\n"+second, map[int]string{7: "present"})
	want := imagePlacement{ID: 7, X: 0, Y: 0, Source: image.Rect(1, 1, 4, 3)}
	if len(placements) != 4 || placements[0] != want {
		t.Fatalf("clipped or duplicate image lost: %+v", placements)
	}
	plain := withoutPlaceholders(first + "\n" + second)
	if strings.ContainsRune(plain, kitty.Placeholder) || lipgloss.Width(plain) != 12 || !strings.Contains(plain, "X") {
		t.Fatalf("placeholder removal changed layout: %q", plain)
	}
}

func TestSixelOutputLifecycle(t *testing.T) {
	t.Setenv("TMUX", "")
	renderer, err := newSixelRenderer()
	if err != nil {
		t.Fatal(err)
	}
	r := renderer.(*sixelRenderer)
	defer func() { _ = r.Close(io.Discard) }()
	img := image.NewRGBA(image.Rect(0, 0, 8, 32))
	for y := range 32 {
		for x := range 8 {
			img.Set(x, y, color.RGBA{R: uint8(y * 7), G: 90, B: 180, A: 255})
		}
	}
	view, err := r.Render("scoped/item", img, 1, 2)
	if err != nil {
		t.Fatal(err)
	}
	// A single 16-pixel row cuts through Sixel's six-pixel bands.
	clipped := strings.Split(view, "\n")[1]
	r.Frame(clipped)
	var output bytes.Buffer
	if _, err := r.Write(&output, []byte(clipped)); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(output.String(), "\x1bP0;1q\"1;1;8;16") || strings.ContainsRune(output.String(), kitty.Placeholder) {
		t.Fatalf("missing cropped Sixel raster or visible placeholders: %q", output.String())
	}
	output.Reset()
	if _, err := r.Write(&output, []byte("\x1b[?25l")); err != nil {
		t.Fatal(err)
	}
	if strings.Contains(output.String(), "\x1bP") {
		t.Fatal("unchanged image retransmitted")
	}
	r.Frame("dialog")
	output.Reset()
	if _, err := r.Write(&output, []byte("dialog")); err != nil {
		t.Fatal(err)
	}
	if strings.Contains(output.String(), "\x1bP") || !strings.Contains(output.String(), "\x1b[2Kdialog") {
		t.Fatal("dialog did not replace the image")
	}
	r.Frame(clipped)
	output.Reset()
	_, _ = r.Write(&output, []byte(clipped))
	if !strings.Contains(output.String(), "\x1bP") {
		t.Fatal("image not restored after dialog closed")
	}
}

func TestRendererSelection(t *testing.T) {
	for _, tt := range []struct {
		name string
		mode string
		caps termgraphics.Capabilities
		want string
	}{
		{name: "ghostty", mode: "auto", caps: termgraphics.Capabilities{Kitty: true, Name: "Ghostty 1.3.1"}, want: "kitty"},
		{name: "sixel terminal", mode: "auto", caps: termgraphics.Capabilities{Sixel: true}, want: "sixel"},
		{name: "unknown", mode: "auto", want: "halfcell"},
		{name: "forced", mode: "sixel", want: "sixel"},
		{name: "fallback requested", mode: "halfcell", caps: termgraphics.Capabilities{Sixel: true}, want: "halfcell"},
	} {
		t.Run(tt.name, func(t *testing.T) {
			r, err := chooseRenderer(tt.mode, tt.caps)
			if err != nil {
				t.Fatal(err)
			}
			defer func() { _ = r.Close(io.Discard) }()
			if r.Name() != tt.want {
				t.Fatalf("renderer = %s, want %s", r.Name(), tt.want)
			}
		})
	}
}
