package termgraphics

import (
	"bytes"
	"fmt"
	"strings"
	"testing"

	"github.com/charmbracelet/x/ansi"
)

func TestVideoAnchorsEveryPacketInTmuxPane(t *testing.T) {
	const cursor = "\x1b[13;1f"
	const first = "\x1b_Ga=T,f=24,s=531,v=298,m=1;AAAA\x1b\\"
	const last = "\x1b_Gm=0;BBBB\x1b\\"
	for split := 1; split <= len(cursor+first+last); split++ {
		var output bytes.Buffer
		w := Writer{Output: &output}
		w.SetPaneOrigin(74, 2)
		input := cursor + first + last
		for at := 0; at < len(input); at += split {
			if _, err := w.Write([]byte(input[at:min(at+split, len(input))])); err != nil {
				t.Fatal(err)
			}
		}
		want := cursor + ansi.TmuxPassthrough("\x1b[15;75H"+first) + ansi.TmuxPassthrough("\x1b[15;75H"+last)
		if output.String() != want {
			t.Fatalf("chunk size %d did not anchor every graphics packet in the pane", split)
		}
		if w.Frame() != (Frame{X: 0, Y: 12, Width: 531, Height: 298}) {
			t.Fatal("mouse coordinates must stay relative to the pane", w.Frame())
		}
		output.Reset()
		if !w.SetPaneOrigin(0, 0) || w.SetPaneOrigin(0, 0) {
			t.Fatal("pane movement was not detected")
		}
		if _, err := w.Write([]byte(cursor + first + last)); err != nil {
			t.Fatal(err)
		}
		want = cursor + ansi.TmuxPassthrough("\x1b[13;1H"+first) + ansi.TmuxPassthrough("\x1b[13;1H"+last)
		if output.String() != want {
			t.Fatal("graphics kept the old pane offset after fullscreen")
		}
	}
}

func TestVideoRetainsPreviousFrameDuringUpload(t *testing.T) {
	const first = "\x1b_Ga=T,f=24,s=2,v=1,m=1;AAAA\x1b\\"
	const middle = "\x1b_Gm=1;CCCC\x1b\\"
	const last = "\x1b_Gm=0;BBBB\x1b\\"
	for _, direct := range []bool{true, false} {
		t.Run(fmt.Sprintf("direct=%v", direct), func(t *testing.T) {
			for split := 1; split <= len(first+last); split++ {
				var output bytes.Buffer
				w := Writer{Output: &output, Direct: direct, ImageID: 100}
				write := func(s string) {
					t.Helper()
					for at := 0; at < len(s); at += split {
						if _, err := w.Write([]byte(s[at:min(at+split, len(s))])); err != nil {
							t.Fatal(err)
						}
					}
				}
				wrap := func(s string) string {
					if direct {
						return s
					}
					return ansi.TmuxPassthrough(s)
				}
				previous := 0
				for frame := range 4 {
					id := 100 + frame%2
					output.Reset()
					write(first)
					// The visible image's ID must not be replaced or deleted while
					// Kitty waits for the rest of a frame from a slow terminal/SSH.
					want := strings.Replace(first, ";", fmt.Sprintf(",i=%d;", id), 1)
					if output.String() != wrap(want) {
						t.Fatalf("split %d, frame %d: upload replaced the visible image", split, frame)
					}
					output.Reset()
					write(middle)
					if output.String() != wrap(middle) {
						t.Fatalf("split %d, frame %d: deleted an image in the middle of a chunked upload", split, frame)
					}
					output.Reset()
					write(last)
					want = last
					if previous != 0 {
						want += DeleteImage(previous)
					}
					if output.String() != wrap(want) {
						t.Fatalf("split %d, frame %d: previous frame not retired after completion", split, frame)
					}
					previous = id
				}
				output.Reset()
				write("\x1b_Ga=d;\x1b\\")
				if output.String() != wrap(DeleteImage(100)+DeleteImage(101)) {
					t.Fatal("resize/exit did not clean up both owned images")
				}
			}
		})
	}
}

func TestSharedMemoryFramesCompleteWithoutContinuation(t *testing.T) {
	var output bytes.Buffer
	w := Writer{Output: &output, Direct: true, ImageID: 100}
	const frame = "\x1b_Ga=T,t=s,f=24,s=2,v=1,m=1;L21wdg==\x1b\\"
	for i := range 3 {
		output.Reset()
		if _, err := w.Write([]byte(frame)); err != nil {
			t.Fatal(err)
		}
		want := strings.Replace(frame, ";", fmt.Sprintf(",i=%d;", 100+i%2), 1)
		if i > 0 {
			want += DeleteImage(100 + (i-1)%2)
		}
		if output.String() != want {
			t.Fatal("shared-memory frame did not replace and retire its predecessor")
		}
	}
}
