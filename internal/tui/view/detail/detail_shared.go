package detail

import (
	"bytes"
	"context"
	"fmt"
	"image"
	"log/slog"
	"strings"
	"time"

	_ "image/jpeg"
	_ "image/png"

	"github.com/LukeHagar/plexgo/models/components"
	"github.com/charmbracelet/bubbles/list"
	"github.com/charmbracelet/bubbles/viewport"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	tint "github.com/lrstanley/bubbletint"
	gopixels "github.com/saran13raj/go-pixels"
	"github.com/ygelfand/plexctl/internal/plex"
	"github.com/ygelfand/plexctl/internal/ui"
)

// Shared Message Types
type detailDataMsg struct {
	metadata *components.Metadata
	children []components.Metadata
}

type posterDataMsg string

type BackToLibraryMsg struct{}

type BackMsg struct{}

// Shared Item Type for Lists
type metadataItem struct {
	metadata components.Metadata
}

func (i metadataItem) Title() string { return i.metadata.Title }
func (i metadataItem) Description() string {
	if i.metadata.Summary != nil {
		return *i.metadata.Summary
	}
	return ""
}
func (i metadataItem) FilterValue() string { return i.metadata.Title }

// Shared Helpers

func fetchPoster(metadata *components.Metadata, width int) tea.Cmd {
	return func() tea.Msg {
		if metadata == nil || metadata.Thumb == nil {
			return nil
		}

		rk := ""
		if metadata.RatingKey != nil {
			rk = *metadata.RatingKey
		}

		targetWidth := ui.DetailPosterWidth

		slog.Debug("fetchPoster start", "title", metadata.Title, "ratingKey", rk, "width", targetWidth)

		// Check long-term cache for rendered string
		if rk != "" {
			if cached, ok := plex.GetCachedPoster(rk, targetWidth); ok {
				return posterDataMsg(cached)
			}
		}

		start := time.Now()
		data, err := plex.GetImage(context.Background(), *metadata.Thumb)
		if err != nil {
			slog.Error("fetchPoster image fetch failed", "error", err)
			return nil
		}
		slog.Debug("fetchPoster image fetched", "duration", time.Since(start))

		start = time.Now()
		img, _, err := image.Decode(bytes.NewReader(data))
		if err != nil {
			slog.Error("fetchPoster decode failed", "error", err)
			return nil
		}

		imgStr, err := gopixels.FromImageStream(img, targetWidth, 0, "halfcell", true)
		if err != nil {
			slog.Error("fetchPoster render failed", "error", err)
			return nil
		}
		slog.Debug("fetchPoster render complete", "duration", time.Since(start))

		// Save to long-term cache
		if rk != "" {
			plex.SetCachedPoster(rk, targetWidth, imgStr)
		}

		return posterDataMsg(imgStr)
	}
}

func formatDuration(ms *int) string {
	if ms == nil {
		return ""
	}
	d := *ms / 1000
	h := d / 3600
	m := (d % 3600) / 60
	if h > 0 {
		return fmt.Sprintf("%dh %dm", h, m)
	}
	return fmt.Sprintf("%dm", m)
}

func renderBadges(metadata *components.Metadata, theme tint.Tint) string {
	badgeStyle := lipgloss.NewStyle().
		Background(ui.Accent(theme)).
		Foreground(lipgloss.Color("#000000")).
		Padding(0, 1).
		MarginRight(1).
		Bold(true)

	var formats []string
	if len(metadata.Media) > 0 {
		media := metadata.Media[0]
		if media.VideoResolution != nil {
			formats = append(formats, badgeStyle.Render(strings.ToUpper(*media.VideoResolution)))
		}
		if media.VideoCodec != nil {
			formats = append(formats, badgeStyle.Render(strings.ToUpper(*media.VideoCodec)))
		}
		if media.AudioCodec != nil {
			formats = append(formats, badgeStyle.Render(strings.ToUpper(*media.AudioCodec)))
		}
		if media.AudioChannels != nil {
			ch := ""
			switch *media.AudioChannels {
			case 2:
				ch = "Stereo"
			case 6:
				ch = "5.1"
			case 8:
				ch = "7.1"
			default:
				ch = fmt.Sprintf("%d ch", *media.AudioChannels)
			}
			formats = append(formats, badgeStyle.Render(ch))
		}
	}
	return lipgloss.JoinHorizontal(lipgloss.Top, formats...)
}

func createBaseDetailList() list.Model {
	l := list.New(nil, list.NewDefaultDelegate(), 0, 0)
	l.SetShowStatusBar(false)
	l.SetFilteringEnabled(false)
	l.SetShowHelp(false)
	return l
}

func createBaseViewport() viewport.Model {
	return viewport.New(0, 0)
}

func RenderWatchedStatus(metadata *components.Metadata) string {
	if metadata == nil {
		return ""
	}

	if metadata.ViewCount != nil && *metadata.ViewCount > 0 {
		return "✓ Watched"
	}

	if metadata.ViewedLeafCount != nil && metadata.LeafCount != nil && *metadata.LeafCount > 0 {
		if *metadata.ViewedLeafCount == *metadata.LeafCount {
			return "✓ Watched"
		}
		return fmt.Sprintf("󱉟 %d/%d", *metadata.ViewedLeafCount, *metadata.LeafCount)
	}

	if metadata.ViewOffset != nil && *metadata.ViewOffset > 0 {
		if metadata.Duration != nil && *metadata.Duration > 0 {
			percent := float64(*metadata.ViewOffset) / float64(*metadata.Duration) * 100
			return fmt.Sprintf("󱉟 %.0f%%", percent)
		}
		return "󱉟 In Progress"
	}

	return ""
}
