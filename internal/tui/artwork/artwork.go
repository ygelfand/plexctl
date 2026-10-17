package artwork

import (
	"fmt"
	"image"
	"io"
	"math/rand/v2"
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"sync"

	"github.com/charmbracelet/x/ansi/kitty"
	"github.com/ygelfand/plexctl/internal/termgraphics"
	"golang.org/x/image/draw"
)

type kittyRenderer struct {
	sync.Mutex
	dir    string
	nextID int
	views  map[string]string
	files  map[int]string
	wanted map[int]bool
	sent   map[int]uint64
	clock  uint64
}

func newKittyRenderer() (Renderer, error) {
	dir, err := os.MkdirTemp("", "plexctl-posters-")
	if err != nil {
		return nil, err
	}
	return &kittyRenderer{
		dir: dir, views: make(map[string]string), files: make(map[int]string),
		wanted: make(map[int]bool), sent: make(map[int]uint64), nextID: 1 + rand.IntN(1<<22),
	}, nil
}

func (*kittyRenderer) Name() string { return "kitty" }

func key(ratingKey string, width, height int) string {
	// Callers include the server and credential namespace in the artwork key.
	return fmt.Sprintf("%s/%d/%d", ratingKey, width, height)
}

func (r *kittyRenderer) Cached(ratingKey string, width, height int) (string, bool) {
	r.Lock()
	defer r.Unlock()
	view, ok := r.views[key(ratingKey, width, height)]
	return view, ok
}

func (r *kittyRenderer) Render(ratingKey string, img image.Image, width, height int) (string, error) {
	r.Lock()
	defer r.Unlock()
	if cached, ok := r.views[key(ratingKey, width, height)]; ok {
		return cached, nil
	}
	dir, id := r.dir, r.nextID
	r.nextID++
	cacheKey := key(ratingKey, width, height)
	if width <= 0 || width > 255 {
		return "", fmt.Errorf("invalid poster width: %d", width)
	}
	if img == nil || img.Bounds().Empty() {
		return "", fmt.Errorf("empty poster image")
	}
	cols, rows := fitImage(img.Bounds(), width, height)
	// Retain hundreds of pixels of detail, including on HiDPI displays, while
	// bounding transport and terminal texture memory for very large originals.
	if longest := max(img.Bounds().Dx(), img.Bounds().Dy()); longest > 768 {
		resized := image.NewRGBA(image.Rect(0, 0,
			max(1, img.Bounds().Dx()*768/longest), max(1, img.Bounds().Dy()*768/longest)))
		draw.CatmullRom.Scale(resized, resized.Bounds(), img, img.Bounds(), draw.Over, nil)
		img = resized
	}
	path := filepath.Join(dir, strconv.Itoa(id))
	f, err := os.Create(path)
	if err != nil {
		return "", err
	}
	err = kitty.EncodeGraphics(f, img, &kitty.Options{
		Action: kitty.TransmitAndPut, Quite: 2, ID: id, Format: kitty.PNG,
		Transmission: kitty.Direct, Chunk: true, ChunkFormatter: termgraphics.Wrap,
		Columns: cols, Rows: rows, VirtualPlacement: true,
	})
	closeErr := f.Close()
	if err != nil {
		return "", err
	}
	if closeErr != nil {
		return "", closeErr
	}
	view := padImage(placeholders(id, cols, rows), width, height)
	r.files[id] = path
	r.views[cacheKey] = view
	return view, nil
}

func placeholders(id, columns, rows int) string {
	var s strings.Builder
	for y := range rows {
		if y > 0 {
			s.WriteByte('\n')
		}
		fmt.Fprintf(&s, "\x1b[38;2;%d;%d;%dm\x1b[59m", id>>16&255, id>>8&255, id&255)
		for x := range columns {
			s.WriteRune(kitty.Placeholder)
			s.WriteRune(kitty.Diacritic(y))
			s.WriteRune(kitty.Diacritic(x))
			s.WriteRune(kitty.Diacritic(0))
		}
		s.WriteString("\x1b[39m")
	}
	return s.String()
}

var imageColor = regexp.MustCompile(`\x1b\[38;2;(\d+);(\d+);(\d+)m`)

// Frame records images in the final, clipped/composited view. The output writer
// sends their payloads before the next screen update, on the renderer's own
// goroutine. Background image fetches never write directly to the terminal.
func (r *kittyRenderer) Frame(view string) string {
	r.Lock()
	defer r.Unlock()
	wanted := make(map[int]bool)
	for _, m := range imageColor.FindAllStringSubmatch(view, -1) {
		red, _ := strconv.Atoi(m[1])
		g, _ := strconv.Atoi(m[2])
		b, _ := strconv.Atoi(m[3])
		id := red<<16 | g<<8 | b
		if _, ok := r.files[id]; ok {
			wanted[id] = true
		}
	}
	r.wanted = wanted
	return view
}

func (r *kittyRenderer) Write(out io.Writer, p []byte) (int, error) {
	r.Lock()
	defer r.Unlock()
	if strings.Contains(string(p), "\x1b[?1049l") {
		// An external player may clear graphics while Bubble Tea is suspended.
		clear(r.sent)
		return out.Write(p)
	}
	for id := range r.wanted {
		r.clock++
		if _, ok := r.sent[id]; !ok {
			data, err := os.ReadFile(r.files[id])
			if err != nil {
				return 0, err
			}
			if _, err := out.Write(data); err != nil {
				return 0, err
			}
		}
		r.sent[id] = r.clock
	}
	// Bound terminal texture memory; inactive posters can be uploaded again.
	for len(r.sent) > max(64, len(r.wanted)) {
		oldest, age := 0, ^uint64(0)
		for id, used := range r.sent {
			if !r.wanted[id] && used < age {
				oldest, age = id, used
			}
		}
		if oldest == 0 {
			break
		}
		_, _ = io.WriteString(out, deleteImage(oldest))
		delete(r.sent, oldest)
	}
	return out.Write(p)
}

func deleteImage(id int) string {
	return termgraphics.Wrap(fmt.Sprintf("\x1b_Ga=d,d=I,i=%d,q=2;\x1b\\", id))
}

func (r *kittyRenderer) Close(out io.Writer) error {
	r.Lock()
	defer r.Unlock()
	for id := range r.files {
		_, _ = io.WriteString(out, deleteImage(id))
	}
	clear(r.wanted)
	clear(r.files)
	if r.dir != "" {
		err := os.RemoveAll(r.dir)
		r.dir = ""
		return err
	}
	return nil
}
