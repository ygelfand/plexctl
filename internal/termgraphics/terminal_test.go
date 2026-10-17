package termgraphics

import (
	"bytes"
	"os"
	"path/filepath"
	"testing"

	"github.com/charmbracelet/x/ansi"
)

func TestTmuxMustOwnCurrentTerminal(t *testing.T) {
	dir := t.TempDir()
	parent, err := os.Create(filepath.Join(dir, "parent-pane"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if err := parent.Close(); err != nil {
			t.Error(err)
		}
	})
	direct, err := os.Create(filepath.Join(dir, "direct-kitty"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if err := direct.Close(); err != nil {
			t.Error(err)
		}
	})
	if sameTerminal(parent.Name(), direct, direct) {
		t.Fatal("an inherited TMUX_PANE redirected a new terminal's graphics")
	}
	if !sameTerminal(parent.Name(), parent, direct) || !sameTerminal(parent.Name(), direct, parent) {
		t.Fatal("the actual tmux pane was not recognized")
	}
	if sameTerminal(filepath.Join(dir, "closed-pane"), direct, direct) {
		t.Fatal("a stale pane was recognized")
	}
}

func TestCapabilities(t *testing.T) {
	for _, tt := range []struct {
		name, reply string
		want        Capabilities
	}{
		{"unknown", "", Capabilities{}},
		{"vte", "\x1b[?61;1;21;22c\x1bP>|VTE(7600)\x1b\\", Capabilities{Responded: true, Name: "VTE(7600)"}},
		{"kitty", "\x1b_Gi=18901;OK\x1b\\\x1b[?62;4;22c", Capabilities{Responded: true, Kitty: true, Sixel: true}},
		{"local kitty", "\x1b_Gi=18901;OK\x1b\\\x1b_Gi=18902;OK\x1b\\", Capabilities{Responded: true, Kitty: true, KittySharedMemory: true}},
		{"remote kitty", "\x1b_Gi=18901;OK\x1b\\\x1b_Gi=18902;EBADF:Failed to read image\x1b\\", Capabilities{Responded: true, Kitty: true}},
		{"sixel", "\x1b[?64;1;4;22c", Capabilities{Responded: true, Sixel: true}},
		{"cell size", "\x1b[6;20;10t\x1b[?64;4c", Capabilities{Responded: true, Sixel: true, CellWidth: 10, CellHeight: 20}},
		{"not four", "\x1b[?64;14;24c", Capabilities{Responded: true}},
	} {
		t.Run(tt.name, func(t *testing.T) {
			if got := parseCapabilities(tt.reply); got != tt.want {
				t.Errorf("got %+v, want %+v", got, tt.want)
			}
		})
	}
}

func TestTmuxPacketsSplitAcrossWrites(t *testing.T) {
	kitty := "\x1b_Ga=T,f=24,s=1,v=1;AAAA\x1b\\"
	sixel := "\x1bPq#0;2;100;0;0~\x1b\\"
	input := "\x1b[?1049h\x1b[2;3H" + kitty + sixel + "\x1b[?1049l"
	want := "\x1b[?1049h\x1b[2;3H" + ansi.TmuxPassthrough(kitty) + ansi.TmuxPassthrough(sixel) + "\x1b[?1049l"
	for split := 1; split <= len(input); split++ {
		var output bytes.Buffer
		w := Writer{Output: &output}
		for at := 0; at < len(input); at += split {
			p := []byte(input[at:min(at+split, len(input))])
			if n, err := w.Write(p); err != nil || n != len(p) {
				t.Fatalf("write: %d, %v", n, err)
			}
		}
		if output.String() != want {
			t.Fatalf("chunk size %d corrupted graphics or cursor movement", split)
		}
	}
}

func TestKittyPlaybackImageOwnership(t *testing.T) {
	var output bytes.Buffer
	w := Writer{Output: &output, ImageID: 1073741824}
	if _, err := w.Write([]byte("\x1b_Ga=d;\x1b\\\x1b_Ga=T,f=24,s=1,v=1;AAAA\x1b\\")); err != nil {
		t.Fatal(err)
	}
	want := ansi.TmuxPassthrough(DeleteImage(1073741824)+DeleteImage(1073741825)) + ansi.TmuxPassthrough("\x1b_Ga=T,f=24,s=1,v=1,i=1073741824;AAAA\x1b\\")
	if output.String() != want {
		t.Fatal("video could overwrite or delete other applications' images")
	}
}

func TestKittyFramePlacement(t *testing.T) {
	input := "\x1b[4;12H\x1b_Ga=T,f=24,s=800,v=450,C=1,q=2,m=1;AAAA\x1b\\\x1b_Gm=0;BBBB\x1b\\"
	for split := 1; split <= len(input); split++ {
		var output bytes.Buffer
		w := Writer{Output: &output, Direct: true}
		for at := 0; at < len(input); at += split {
			if _, err := w.Write([]byte(input[at:min(at+split, len(input))])); err != nil {
				t.Fatal(err)
			}
		}
		if w.Frame() != (Frame{X: 11, Y: 3, Width: 800, Height: 450}) {
			t.Fatalf("split %d: incorrect frame %+v", split, w.Frame())
		}
		if output.String() != input {
			t.Fatalf("split %d: direct graphics were altered", split)
		}
		if _, err := w.Write([]byte("\x1b[0;0H\x1b_Ga=T,f=24,s=400,v=225;AAAA\x1b\\")); err != nil {
			t.Fatal(err)
		}
		if w.Frame() != (Frame{Width: 400, Height: 225}) {
			t.Fatal("frame was not updated after resize")
		}
	}
}

func TestNativePostersRequireUnicodePlaceholders(t *testing.T) {
	t.Setenv("TERM_PROGRAM", "")
	t.Setenv("TERM", "")
	for _, tt := range []struct {
		name string
		want bool
	}{
		{"kitty(0.27.0)", false}, {"kitty(0.28.0)", true}, {"kitty(0.49.2)", true}, {"kitty(1.0.0)", true}, {"VTE(7600)", false}, {"", false},
		{"ghostty 1.2.0", true}, {"Ghostty(1.3.0)", true}, {"WezTerm 20240203", false},
	} {
		if got := (Capabilities{Kitty: true, Name: tt.name}).NativePosters(); got != tt.want {
			t.Errorf("%s: got %v", tt.name, got)
		}
	}
	t.Setenv("TERM_PROGRAM", "ghostty")
	if !(Capabilities{Kitty: true}).NativePosters() || (Capabilities{}).NativePosters() {
		t.Fatal("Ghostty environment fallback must still require a successful graphics probe")
	}
}
