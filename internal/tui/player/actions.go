package player

import (
	"context"
	"fmt"
	"log/slog"
	"strings"
	"time"

	"github.com/LukeHagar/plexgo/models/components"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/ygelfand/plexctl/internal/config"
	"github.com/ygelfand/plexctl/internal/plex"
	"github.com/ygelfand/plexctl/internal/ui"
)

// PlayItem carries the Plex identity of one playlist entry. The queue is kept
// in playlist order rather than keyed by URL, since a playlist can repeat files.
type PlayItem struct {
	URL       string
	Title     string
	RatingKey string
	Duration  float64
}

type PlayRequest struct {
	Items     []PlayItem
	Server    config.ServerTarget
	NoReport  bool
	Music     bool
	Subtitles []ExternalSubtitle
}

// BuildPlayRequest resolves containers before starting mpv, so an unsuccessful
// library request cannot interrupt the currently playing item.
func BuildPlayRequest(ctx context.Context, metadata *components.Metadata, noReport bool) (*PlayRequest, error) {
	ctx, cancel := context.WithTimeout(ctx, 2*time.Minute)
	defer cancel()
	server := config.ServerFromContext(ctx)
	if server.URL == "" {
		return nil, fmt.Errorf("no active server")
	}
	if metadata == nil {
		return nil, fmt.Errorf("no playable media found")
	}
	tracks := []components.Metadata{*metadata}
	if metadata.Type == "artist" || metadata.Type == "album" {
		var err error
		tracks, err = plex.CollectTracks(ctx, metadata)
		if err != nil {
			return nil, err
		}
	}
	req := &PlayRequest{Server: server, NoReport: noReport, Music: config.IsMusic(metadata.Type)}
	for _, track := range tracks {
		part := plex.PlayablePart(&track)
		if part == nil {
			return nil, fmt.Errorf("no playable media found")
		}
		mediaURL, err := plex.MediaURL(server, part.Key)
		if err != nil {
			return nil, fmt.Errorf("invalid media path: %w", err)
		}
		item := PlayItem{URL: mediaURL, Title: track.Title}
		if track.RatingKey != nil {
			item.RatingKey = *track.RatingKey
		}
		if track.Duration != nil {
			item.Duration = float64(*track.Duration) / 1000
		}
		if track.Type == "track" && track.GrandparentTitle != nil {
			item.Title = *track.GrandparentTitle + " — " + item.Title
		}
		if noReport {
			item.Title = "Trailer: " + item.Title
		}
		req.Items = append(req.Items, item)
		if len(tracks) != 1 {
			continue
		}
		for _, stream := range part.Stream {
			if stream.StreamType != components.StreamTypeSubtitle || stream.Key == "" {
				continue
			}
			language := ""
			if stream.LanguageCode != nil {
				language = *stream.LanguageCode
			}
			subtitleURL, err := plex.MediaURL(server, stream.Key)
			if err != nil {
				return nil, fmt.Errorf("invalid subtitle path: %w", err)
			}
			req.Subtitles = append(req.Subtitles, ExternalSubtitle{
				URL: subtitleURL, Title: stream.DisplayTitle, Language: language,
			})
		}
	}
	return req, nil
}

func PlayMedia(parent context.Context, metadata *components.Metadata, noReport bool, output config.VideoOutput, startOffset int64, servers ...config.ServerTarget) tea.Cmd {
	server := config.MediaServer(servers...)
	pm := GetPlayerManager()
	ctx := pm.beginRequest(server.Context(parent))
	return pm.resolveAndPlay(ctx, metadata, noReport, output, startOffset)
}

func (pm *PlayerManager) resolveAndPlay(ctx context.Context, metadata *components.Metadata, noReport bool, output config.VideoOutput, startOffset int64) tea.Cmd {
	return func() tea.Msg {
		req, err := BuildPlayRequest(ctx, metadata, noReport)
		if err != nil {
			if ctx.Err() == context.Canceled {
				return nil
			}
			return err
		}
		return pm.playRequest(ctx, req, output, startOffset)()
	}
}

// FetchAndPlay handles the full playback logic: fetch full metadata, check for resume, then play
func FetchAndPlay(ratingKey string, output config.VideoOutput, servers ...config.ServerTarget) tea.Cmd {
	server := config.MediaServer(servers...)
	pm := GetPlayerManager()
	ctx := pm.beginRequest(server.Context(context.Background()))
	return func() tea.Msg {
		metadataCtx, cancel := context.WithTimeout(ctx, 2*time.Minute)
		defer cancel()
		meta, err := plex.GetMetadata(metadataCtx, ratingKey, true)
		if ctx.Err() == context.Canceled {
			return nil
		}
		if err != nil {
			return err
		}

		if meta.Type != "artist" && meta.Type != "album" && meta.ViewOffset != nil && *meta.ViewOffset > 0 {
			return ui.ResumeChoiceMsg{Server: server,
				Metadata: meta,
				Output:   output,
			}
		}

		return pm.resolveAndPlay(ctx, meta, false, output, 0)()
	}
}

// FetchAndPlayTrailer handles finding and playing the correct trailer for a media item
func FetchAndPlayTrailer(parent context.Context, parentMeta *components.Metadata, output config.VideoOutput, servers ...config.ServerTarget) tea.Cmd {
	server := config.MediaServer(servers...)
	pm := GetPlayerManager()
	ctx := pm.beginRequest(server.Context(parent))
	return func() tea.Msg {
		targetKey := ""
		if parentMeta.PrimaryExtraKey != nil {
			targetKey = *parentMeta.PrimaryExtraKey
		} else if parentMeta.Extras != nil {
			for _, extra := range parentMeta.Extras.Metadata {
				if extra.Subtype != nil && *extra.Subtype == "trailer" {
					if extra.RatingKey != nil {
						targetKey = *extra.RatingKey
						break
					}
				}
			}
		}

		if targetKey == "" {
			return fmt.Errorf("no trailer found for this item")
		}

		// Normalize key (some extra keys are full paths)
		parts := strings.Split(targetKey, "/")
		ratingKey := parts[len(parts)-1]

		slog.Debug("FetchAndPlayTrailer: Resolved trailer key", "parent", parentMeta.Title, "target", ratingKey)

		metadataCtx, cancel := context.WithTimeout(ctx, 2*time.Minute)
		defer cancel()
		meta, err := plex.GetMetadata(metadataCtx, ratingKey, false)
		if ctx.Err() == context.Canceled {
			return nil
		}
		if err != nil {
			return err
		}

		return pm.resolveAndPlay(ctx, meta, true, output, 0)()
	}
}
