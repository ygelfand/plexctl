package poster

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/LukeHagar/plexgo/models/components"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	tint "github.com/lrstanley/bubbletint"
	"github.com/ygelfand/plexctl/internal/config"
	"github.com/ygelfand/plexctl/internal/plex"
	"github.com/ygelfand/plexctl/internal/tui/artwork"
	"github.com/ygelfand/plexctl/internal/ui"
)

type PosterItem struct {
	Metadata components.Metadata
	Poster   string
	Loading  bool
	Fetching bool
}

type PosterLoadedMsg struct {
	ServerID  string
	ListID    int
	Index     int
	View      string
	RatingKey string
}

type ItemSelectedMsg struct {
	Metadata components.Metadata
}

// FetchPoster returns a command to fetch and render a poster
func FetchPoster(listID int, index int, metadata components.Metadata, servers ...config.ServerTarget) tea.Cmd {
	server := config.MediaServer(servers...)
	return func() tea.Msg {
		rk := ""
		if metadata.RatingKey != nil {
			rk = *metadata.RatingKey
		}
		result := PosterLoadedMsg{ServerID: server.CacheNamespace(), ListID: listID, Index: index, RatingKey: rk}

		if rk != "" {
			if cached, ok := artwork.Cached(server.CacheNamespace()+"/"+rk, ui.PosterWidth, ui.PosterHeight); ok {
				return PosterLoadedMsg{ServerID: server.CacheNamespace(), ListID: listID, Index: index, RatingKey: rk, View: cached}
			}
		}

		path := ""
		if metadata.GrandparentThumb != nil {
			path = *metadata.GrandparentThumb
		} else if metadata.ParentThumb != nil {
			path = *metadata.ParentThumb
		} else if metadata.Thumb != nil {
			path = *metadata.Thumb
		}
		if (metadata.Type == "artist" || metadata.Type == "album") && metadata.Thumb != nil {
			path = *metadata.Thumb
		}
		if metadata.Type == "track" && metadata.ParentThumb != nil {
			path = *metadata.ParentThumb
		}

		if path == "" {
			return result
		}

		ctx, cancel := context.WithTimeout(server.Context(context.Background()), 10*time.Second)
		defer cancel()
		data, err := plex.GetImage(ctx, path)
		if err != nil {
			return result
		}

		img, err := artwork.Decode(data)
		if err != nil {
			return result
		}

		imgStr, err := artwork.Render(server.CacheNamespace()+"/"+rk, img, ui.PosterWidth, ui.PosterHeight)
		if err != nil {
			return result
		}

		return PosterLoadedMsg{ServerID: server.CacheNamespace(), ListID: listID, Index: index, RatingKey: rk, View: imgStr}
	}
}

// RenderPosterItem renders a single poster with its labels
func RenderPosterItem(item *PosterItem, isSelected bool, active bool, theme tint.Tint) string {
	content := item.Poster
	if item.Loading || content == "" {
		title := ui.Ellipsis(item.Metadata.Title, ui.PosterWidth-2)
		content = "\n\n" + title
	}

	accent := ui.Accent(theme)

	boxStyle := lipgloss.NewStyle().
		Border(lipgloss.RoundedBorder()).
		BorderForeground(theme.BrightBlack()).
		Width(ui.PosterWidth).
		Height(ui.PosterHeight).
		Align(lipgloss.Center, lipgloss.Center)

	if isSelected && active {
		boxStyle = boxStyle.BorderForeground(accent)
	}

	label := GetDisplayTitle(&item.Metadata)
	labelStyle := lipgloss.NewStyle().
		Width(ui.PosterWidth).
		Height(2).
		Align(lipgloss.Center).
		Foreground(theme.White()).
		MaxHeight(2)

	if isSelected && active {
		labelStyle = labelStyle.Bold(true).Foreground(accent)
		label = ui.Ellipsis(label, ui.PosterWidth*2)
	} else {
		label = ui.Ellipsis(label, ui.PosterWidth)
	}

	watched := ""
	if item.Metadata.Type == "album" && item.Metadata.Year != nil {
		watched = lipgloss.NewStyle().Foreground(theme.BrightBlack()).Render(fmt.Sprint(*item.Metadata.Year))
	} else if !isSelected || !active {
		watched = RenderMiniWatched(&item.Metadata, theme)
	}

	itemView := lipgloss.JoinVertical(lipgloss.Center,
		boxStyle.Render(content),
		labelStyle.Render(label),
		watched,
	)

	return lipgloss.NewStyle().MarginRight(ui.PosterMargin).Render(itemView)
}

func GetDisplayTitle(meta *components.Metadata) string {
	if meta.Type != "episode" {
		return meta.Title
	}

	season := 0
	if meta.ParentIndex != nil {
		season = int(*meta.ParentIndex)
	}
	episode := 0
	if meta.Index != nil {
		episode = int(*meta.Index)
	}
	s00e00 := fmt.Sprintf("S%02dE%02d", season, episode)

	showTitle := ""
	if meta.GrandparentTitle != nil {
		showTitle = *meta.GrandparentTitle
	}

	full := fmt.Sprintf("%s %s", showTitle, s00e00)
	if lipgloss.Width(full) > ui.PosterWidth*2 {
		return s00e00
	}
	return full
}

func RenderMiniWatched(meta *components.Metadata, theme tint.Tint) string {
	if meta.ViewOffset != nil && *meta.ViewOffset > 0 && meta.Duration != nil && *meta.Duration > 0 {
		percent := min(1.0, float64(*meta.ViewOffset)/float64(*meta.Duration))
		width := ui.PosterWidth - 2
		filled := int(float64(width) * percent)
		if filled < 1 && percent > 0 {
			filled = 1
		}
		empty := max(width-filled, 0)

		accent := ui.Accent(theme)
		bar := lipgloss.NewStyle().Foreground(accent).Render(strings.Repeat("━", filled)) +
			lipgloss.NewStyle().Foreground(theme.BrightBlack()).Render(strings.Repeat("━", empty))
		return bar
	}

	if meta.ViewCount != nil && *meta.ViewCount > 0 {
		return lipgloss.NewStyle().Foreground(theme.BrightGreen()).Render("✓")
	}

	return ""
}
