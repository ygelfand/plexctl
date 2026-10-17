package poster

import (
	"math"
	"testing"

	"github.com/LukeHagar/plexgo/models/components"
	"github.com/charmbracelet/lipgloss"
	"github.com/ygelfand/plexctl/internal/ui"
)

func TestMiniWatchedBoundsMalformedOffsets(t *testing.T) {
	for _, tt := range []struct {
		name             string
		offset, duration int
	}{
		{"in progress", 5, 10},
		{"past end", 100, 10},
		{"huge offset", math.MaxInt, 1},
	} {
		t.Run(tt.name, func(t *testing.T) {
			meta := &components.Metadata{ViewOffset: &tt.offset, Duration: &tt.duration}
			bar := RenderMiniWatched(meta, ui.PlexctlTheme)
			if width := lipgloss.Width(bar); width != ui.PosterWidth-2 {
				t.Fatalf("metadata generated a %d-cell bar, want %d", width, ui.PosterWidth-2)
			}
		})
	}
}
