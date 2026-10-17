package ui

import (
	"strings"

	"github.com/charmbracelet/lipgloss"
	"github.com/charmbracelet/x/ansi"
)

// Overlay composites the 'overlay' string on top of the 'base' string,
// centered both horizontally and vertically.
// It avoids ANSI corruption by completely replacing the horizontal rows
// occupied by the overlay.
func Overlay(base, overlay string, width, height int) string {
	if base == "" {
		return overlay
	}

	// 1. Get dimensions of the overlay content
	overlayHeight := lipgloss.Height(overlay)

	// 2. Calculate start vertical position
	startY := max(0, (height-overlayHeight)/2)

	baseLines := strings.Split(base, "\n")

	overlay = lipgloss.NewStyle().Width(lipgloss.Width(overlay)).Render(overlay)
	overlayLines := strings.Split(overlay, "\n")

	// Ensure base has enough lines to fill the screen
	for len(baseLines) < height {
		baseLines = append(baseLines, strings.Repeat(" ", width))
	}

	result := make([]string, len(baseLines))
	copy(result, baseLines)

	for y, oLine := range overlayLines {
		baseY := startY + y
		if baseY < 0 || baseY >= len(baseLines) {
			continue
		}

		// Replace the entire background line with a new line that centers the overlay.
		// This is "un-smart" but ANSI-safe because we aren't splicing into the
		// background's escape sequences.
		result[baseY] = lipgloss.NewStyle().
			Width(width).
			Align(lipgloss.Center).
			Render(ansi.Truncate(oLine, width, ""))
	}

	return strings.Join(result, "\n")
}

// Ellipsis truncates a string to a max width and adds ... if needed.
func Ellipsis(s string, maxWidth int) string {
	w := ansi.StringWidth(s)
	if w <= maxWidth {
		return s
	}
	if maxWidth <= 3 {
		return strings.Repeat(".", max(0, maxWidth))
	}
	return ansi.Truncate(s, maxWidth, "...")
}
