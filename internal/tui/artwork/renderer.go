// Package artwork composes terminal posters with Bubble Tea's text layout.
package artwork

import (
	"fmt"
	"image"
	"io"
	"math"
	"os"
	"sync"

	"github.com/charmbracelet/lipgloss"
	gopixels "github.com/saran13raj/go-pixels"
	"github.com/ygelfand/plexctl/internal/plex"
	"github.com/ygelfand/plexctl/internal/termgraphics"
)

// Renderer owns protocol state. Frame sees the composited, clipped view; Write
// runs on Bubble Tea's renderer goroutine, keeping image and text writes atomic.
type Renderer interface {
	Name() string
	Cached(key string, width, height int) (string, bool)
	Render(key string, img image.Image, width, height int) (string, error)
	Frame(view string) string
	Write(out io.Writer, p []byte) (int, error)
	Close(out io.Writer) error
}

var renderers = []struct {
	name      string
	supported func(termgraphics.Capabilities) bool
	build     func() (Renderer, error)
}{
	{"kitty", func(c termgraphics.Capabilities) bool { return c.NativePosters() }, newKittyRenderer},
	{"sixel", func(c termgraphics.Capabilities) bool { return c.Sixel }, newSixelRenderer},
	{"halfcell", func(termgraphics.Capabilities) bool { return true }, func() (Renderer, error) { return halfcellRenderer{}, nil }},
}

var active = struct {
	sync.RWMutex
	renderer Renderer
	output   *Output
	caps     termgraphics.Capabilities
}{renderer: halfcellRenderer{}}

// Open preserves the explicit Kitty/halfcell entry point for embedders.
func Open(file *os.File, kitty bool) (*Output, error) {
	mode := "halfcell"
	if kitty {
		mode = "kitty"
	}
	return OpenRenderer(file, mode, termgraphics.Current())
}

func OpenRenderer(file *os.File, mode string, caps termgraphics.Capabilities) (*Output, error) {
	active.Lock()
	defer active.Unlock()
	renderer, err := chooseRenderer(mode, caps)
	if err != nil {
		return nil, err
	}
	if active.output != nil {
		_ = active.renderer.Close(active.output.file)
	}
	active.renderer, active.caps = renderer, caps
	active.output = &Output{file: file}
	return active.output, nil
}

func chooseRenderer(mode string, caps termgraphics.Capabilities) (Renderer, error) {
	for _, factory := range renderers {
		if mode == factory.name || ((mode == "auto" || mode == "") && factory.supported(caps)) {
			return factory.build()
		}
	}
	return nil, fmt.Errorf("invalid poster renderer %q: choose auto, kitty, sixel, or halfcell", mode)
}

// Select applies a settings change while existing fetches finish under the old
// renderer's read lock. UI consumers reload their poster strings after changing.
func Select(mode string) error {
	active.Lock()
	defer active.Unlock()
	renderer, err := chooseRenderer(mode, active.caps)
	if err != nil {
		return err
	}
	if renderer.Name() == active.renderer.Name() {
		return renderer.Close(io.Discard)
	}
	if active.output != nil {
		if err := active.renderer.Close(active.output.file); err != nil {
			_ = renderer.Close(io.Discard)
			return err
		}
	}
	active.renderer = renderer
	return nil
}

func Name() string {
	active.RLock()
	defer active.RUnlock()
	return active.renderer.Name()
}

func Cached(key string, width int, heights ...int) (string, bool) {
	height := 0
	if len(heights) > 0 {
		height = heights[0]
	}
	active.RLock()
	defer active.RUnlock()
	return active.renderer.Cached(key, width, height)
}

func Render(key string, img image.Image, width, height int) (string, error) {
	if img == nil || img.Bounds().Empty() || width < 1 {
		return "", fmt.Errorf("poster requires a nonempty image and positive width")
	}
	active.RLock()
	defer active.RUnlock()
	return active.renderer.Render(key, img, width, height)
}

func Frame(view string) string {
	active.RLock()
	defer active.RUnlock()
	return active.renderer.Frame(view)
}

type Output struct {
	file *os.File
	mu   sync.Mutex
}

func (o *Output) Fd() uintptr                { return o.file.Fd() }
func (o *Output) Read(p []byte) (int, error) { return o.file.Read(p) }
func (o *Output) TerminalFile() *os.File     { return o.file }
func (o *Output) Write(p []byte) (int, error) {
	active.RLock()
	defer active.RUnlock()
	o.mu.Lock()
	defer o.mu.Unlock()
	return active.renderer.Write(o.file, p)
}
func (o *Output) Close() error {
	active.Lock()
	defer active.Unlock()
	if active.output != o {
		return nil
	}
	o.mu.Lock()
	defer o.mu.Unlock()
	err := active.renderer.Close(o.file)
	active.renderer, active.output = halfcellRenderer{}, nil
	return err
}

// VideoOutput serializes complete video packets with the UI renderer without
// treating video data as a new poster frame. The wrapper owns cursor saving.
func VideoOutput() io.Writer { return videoOutput{} }

type videoOutput struct{}

func (videoOutput) Write(p []byte) (int, error) {
	active.RLock()
	defer active.RUnlock()
	if active.output == nil {
		return 0, fmt.Errorf("terminal video overlay requires a running TUI")
	}
	active.output.mu.Lock()
	defer active.output.mu.Unlock()
	return active.output.file.Write(p)
}

type halfcellRenderer struct{}

func (halfcellRenderer) Name() string { return "halfcell" }
func (halfcellRenderer) Cached(id string, width, height int) (string, bool) {
	return plex.GetCachedPoster("halfcell/v2/"+key(id, width, height), width)
}
func (r halfcellRenderer) Render(id string, img image.Image, width, height int) (string, error) {
	cols, rows := fitImageWithCells(img.Bounds(), width, height, 1, 2)
	view, err := gopixels.FromImageStream(img, cols, rows*2, "halfcell", true)
	if err != nil {
		return "", err
	}
	view = padImage(view, width, height)
	plex.SetCachedPoster("halfcell/v2/"+key(id, width, height), width, view)
	return view, nil
}
func (halfcellRenderer) Frame(view string) string                   { return view }
func (halfcellRenderer) Write(out io.Writer, p []byte) (int, error) { return out.Write(p) }
func (halfcellRenderer) Close(io.Writer) error                      { return nil }

func fitImage(bounds image.Rectangle, width, height int) (int, int) {
	caps := termgraphics.Current()
	cw, ch := caps.CellWidth, caps.CellHeight
	if cw <= 0 || ch <= 0 {
		cw, ch = 8, 16
	}
	return fitImageWithCells(bounds, width, height, cw, ch)
}

func fitImageWithCells(bounds image.Rectangle, width, height, cw, ch int) (int, int) {
	cols := max(1, min(width, 255))
	rows := max(1, int(math.Round(float64(bounds.Dy()*cols*cw)/float64(bounds.Dx()*ch))))
	if height > 0 && rows > height {
		rows = height
		cols = max(1, min(cols, int(math.Round(float64(bounds.Dx()*rows*ch)/float64(bounds.Dy()*cw)))))
	}
	return cols, min(rows, 255)
}

func padImage(view string, width, height int) string {
	return lipgloss.NewStyle().Width(width).Height(height).Align(lipgloss.Center, lipgloss.Center).Render(view)
}
