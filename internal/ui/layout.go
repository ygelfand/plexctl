package ui

import (
	"fmt"
	"sync"

	tint "github.com/lrstanley/bubbletint"
)

const (
	// Sidebar
	SidebarWidth      = 25
	SidebarBorder     = 1
	SidebarTotalWidth = SidebarWidth + SidebarBorder

	// Main Content Container Overhead
	// 1 border char per side (2) + 1 padding char per side (2) = 4
	MainOverhead = 4

	// Poster Geometry
	PosterWidth  = 15
	PosterHeight = 10
	PosterBorder = 2 // 1 char per side
	PosterMargin = 2 // MarginRight(2)

	// PosterStepWidth is the exact horizontal footprint of one poster + border + margin
	PosterStepWidth = PosterWidth + PosterBorder + PosterMargin

	// PosterTotalHeight is: box(10) + border(2) + title(2) + progress(1) = 15
	PosterTotalHeight = PosterHeight + PosterBorder + 3

	// Detail View Geometry
	DetailPosterWidth  = 25
	DetailPosterBorder = 2
	DetailColumnGap    = 4
)

type LayoutManager struct {
	mu           sync.RWMutex
	totalWidth   int
	totalHeight  int
	playerActive bool
	theme        tint.Tint
}

var (
	layoutInstance *LayoutManager
	layoutOnce     sync.Once
)

func GetLayout() *LayoutManager {
	layoutOnce.Do(func() {
		layoutInstance = &LayoutManager{
			theme: PlexctlTheme,
		}
	})
	return layoutInstance
}

func (l *LayoutManager) Update(width, height int, playerActive bool) {
	l.mu.Lock()
	defer l.mu.Unlock()
	l.totalWidth = width
	l.totalHeight = height
	l.playerActive = playerActive
}

func (l *LayoutManager) SetTheme(t tint.Tint) {
	l.mu.Lock()
	defer l.mu.Unlock()
	l.theme = t
}

func (l *LayoutManager) Theme() tint.Tint {
	l.mu.RLock()
	defer l.mu.RUnlock()
	if l.theme == nil {
		return PlexctlTheme
	}
	return l.theme
}

func (l *LayoutManager) TotalWidth() int {
	l.mu.RLock()
	defer l.mu.RUnlock()
	return l.totalWidth
}

func (l *LayoutManager) TotalHeight() int {
	l.mu.RLock()
	defer l.mu.RUnlock()
	return l.totalHeight
}

// MainAreaContentWidth returns the usable width for content INSIDE the main container
func (l *LayoutManager) MainAreaContentWidth() int {
	l.mu.RLock()
	defer l.mu.RUnlock()
	// Calculate total overhead: Sidebar + Content Box Borders/Padding
	overhead := SidebarTotalWidth + MainOverhead
	return max(l.totalWidth-overhead, 0)
}

// InnerWidth is the usable space for widgets
func (l *LayoutManager) InnerWidth() int {
	return l.MainAreaContentWidth()
}

// ContentHeight returns the vertical space available for the active tab content
func (l *LayoutManager) ContentHeight() int {
	l.mu.RLock()
	defer l.mu.RUnlock()
	// -2 for main container horizontal borders, -1 for footer
	h := max(l.totalHeight-3, 0)
	if l.playerActive {
		h -= 4 // Space for player bar
	}
	return h
}

// PosterColumns calculates exactly how many full posters fit in the current inner width
func (l *LayoutManager) PosterColumns() int {
	iw := l.InnerWidth()
	if iw <= 0 {
		return 1
	}
	// We can fit N items if (N * PosterStepWidth) <= iw
	cols := iw / PosterStepWidth
	if cols <= 0 {
		return 1
	}
	return cols
}

// DetailRightColumnWidth returns the width for the info section in detail views
func (l *LayoutManager) DetailRightColumnWidth(hasPoster bool) int {
	// Detail views add an extra Padding(1, 2) in their View() = 4 chars overhead
	usable := max(l.InnerWidth()-4, 0)
	if hasPoster {
		// Poster(25) + Border(2) + Gap(4)
		posterFootprint := DetailPosterWidth + DetailPosterBorder + DetailColumnGap
		return max(usable-posterFootprint, 0)
	}
	return usable
}

// FormatDuration converts milliseconds to a human-readable H:MM:SS or M:SS string
func FormatDuration(ms int) string {
	seconds := ms / 1000
	minutes := seconds / 60
	hours := minutes / 60

	if hours > 0 {
		return fmt.Sprintf("%d:%02d:%02d", hours, minutes%60, seconds%60)
	}
	return fmt.Sprintf("%d:%02d", minutes, seconds%60)
}
