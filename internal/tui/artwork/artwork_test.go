package artwork

import (
	"bytes"
	"encoding/base64"
	"image"
	"image/png"
	"os"
	"regexp"
	"strings"
	"testing"

	"github.com/charmbracelet/lipgloss"
	"github.com/charmbracelet/x/ansi/kitty"
	term "github.com/charmbracelet/x/term"
)

var _ term.File = (*Output)(nil)

func TestNativePosterLifecycle(t *testing.T) {
	t.Setenv("TMUX", "")
	f, err := os.CreateTemp(t.TempDir(), "terminal")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if err := f.Close(); err != nil {
			t.Error(err)
		}
	})
	w, err := Open(f, true)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if err := w.Close(); err != nil {
			t.Error(err)
		}
	})
	view, err := Render("test", image.NewRGBA(image.Rect(0, 0, 600, 900)), 15, 10)
	if err != nil {
		t.Fatal(err)
	}
	if lipgloss.Width(view) != 15 || lipgloss.Height(view) != 10 || !strings.ContainsRune(view, kitty.Placeholder) {
		t.Fatal("incorrect placeholder dimensions")
	}
	frame := Frame(lipgloss.NewStyle().Border(lipgloss.RoundedBorder()).Render(view))
	if len(active.renderer.(*kittyRenderer).wanted) != 1 {
		t.Fatal("image lost when composing frame")
	}
	if _, err := w.Write([]byte(frame)); err != nil {
		t.Fatal(err)
	}
	data, err := os.ReadFile(f.Name())
	if err != nil {
		t.Fatal(err)
	}
	var encoded strings.Builder
	for _, m := range regexp.MustCompile(`\x1b_G[^;]*;([^\x1b]*)\x1b\\`).FindAllSubmatch(data, -1) {
		encoded.Write(m[1])
	}
	pixels, err := base64.StdEncoding.DecodeString(encoded.String())
	if err != nil {
		t.Fatal(err)
	}
	dimensions, err := png.DecodeConfig(bytes.NewReader(pixels))
	if err != nil {
		t.Fatal(err)
	}
	if dimensions.Width != 512 || dimensions.Height != 768 {
		t.Fatalf("poster lost image detail: %+v", dimensions)
	}
	size := func() int64 {
		t.Helper()
		info, err := f.Stat()
		if err != nil {
			t.Fatal(err)
		}
		return info.Size()
	}
	before := size()
	if _, err := w.Write([]byte(frame)); err != nil {
		t.Fatal(err)
	}
	if size()-before != int64(len(frame)) {
		t.Fatal("poster uploaded again without a change")
	}
	if _, err := w.Write([]byte("\x1b[?1049l")); err != nil {
		t.Fatal(err)
	}
	before = size()
	if _, err := w.Write([]byte(frame)); err != nil {
		t.Fatal(err)
	}
	if size()-before <= int64(len(frame)) {
		t.Fatal("poster not reuploaded after returning from playback")
	}
	dir := active.renderer.(*kittyRenderer).dir
	if err := w.Close(); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(dir); !os.IsNotExist(err) {
		t.Fatal("temporary poster data left behind")
	}
}
