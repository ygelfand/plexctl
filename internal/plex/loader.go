package plex

import (
	"context"
	"fmt"
	"log/slog"
	"time"

	"github.com/LukeHagar/plexgo/models/operations"
	"github.com/ygelfand/plexctl/internal/cache"
	"github.com/ygelfand/plexctl/internal/config"
	"github.com/ygelfand/plexctl/internal/ui"
)

type LibraryInfo struct {
	ID    string `json:"id"`
	Title string `json:"title"`
	Type  string `json:"type"`
	Count int    `json:"count"`
}

type LoaderResult struct {
	ServerID  string
	Libraries []LibraryInfo
}

type ProgressUpdate struct {
	Percent float64
	Message string
}

const (
	LibraryCacheTTL = 5 * time.Minute
)

// LoadData performs the initial data fetch and caching.
// It sends progress updates through the provided channel.
func LoadData(ctx context.Context, updates chan<- interface{}) {
	sendUpdate := func(p float64, msg string) {
		if updates != nil {
			updates <- ProgressUpdate{Percent: p, Message: msg}
		}
	}

	cfg := config.Get()
	serverID, _, ok := cfg.GetActiveServer()
	if !ok {
		slog.Error("Loader: No active server configured")
		updates <- fmt.Errorf("no active server configured")
		return
	}

	cm, err := cache.Get(cfg.CacheDir)
	if err != nil {
		slog.Error("Loader: Failed to get cache manager", "error", err)
		updates <- err
		return
	}

	sendUpdate(0.1, "Connecting to Plex...")
	slog.Debug("Loader: Initializing Plex client")
	client, err := NewClient()
	if err != nil {
		slog.Error("Loader: Client initialization failed", "error", err)
		updates <- err
		return
	}

	// 1. Get libraries list (Sections)
	sendUpdate(0.2, "Fetching libraries list...")
	slog.Debug("Loader: Fetching sections from server", "server_id", serverID)
	var sectionsBody operations.GetSectionsResponseBody
	err = cache.AutoCache(cm, serverID, nil, LibraryCacheTTL, &sectionsBody, func() (*operations.GetSectionsResponseBody, error) {
		slog.Log(context.Background(), config.LevelTrace, "Loader: Sections Cache MISS, calling SDK")
		res, err := client.SDK.Library.GetSections(ctx)
		if err != nil {
			return nil, err
		}
		if res.Object == nil {
			return nil, fmt.Errorf("no data in sections response")
		}
		return res.Object, nil
	})

	if err != nil {
		slog.Error("Loader: Failed to get sections", "error", err)
		updates <- err
		return
	}

	if sectionsBody.MediaContainer == nil {
		slog.Error("Loader: No libraries found on server")
		updates <- fmt.Errorf("no libraries found on server")
		return
	}

	dirs := sectionsBody.MediaContainer.Directory
	total := len(dirs)
	slog.Debug("Loader: Found sections", "count", total)
	libraries := make([]LibraryInfo, 0, total)

	for i, dir := range dirs {
		key := ui.PtrToString(dir.Key)
		title := ui.PtrToString(dir.Title)

		slog.Debug("Loader: Processing library", "title", title, "index", i+1, "total", total)
		p := 0.2 + (float64(i+1)/float64(total))*0.7
		msg := fmt.Sprintf("Loading %s...", title)
		sendUpdate(p, msg)

		var libInfo LibraryInfo
		req := operations.ListContentRequest{SectionID: key}
		err = cache.AutoCache(cm, serverID, req, LibraryCacheTTL, &libInfo, func() (*LibraryInfo, error) {
			slog.Log(context.Background(), config.LevelTrace, "Loader: Content Cache MISS", "library", title)
			count := 0
			contentRes, err := client.SDK.Content.ListContent(ctx, req)
			if err == nil &&
				contentRes.MediaContainerWithMetadata != nil &&
				contentRes.MediaContainerWithMetadata.MediaContainer != nil {
				if contentRes.MediaContainerWithMetadata.MediaContainer.TotalSize != nil {
					count = int(*contentRes.MediaContainerWithMetadata.MediaContainer.TotalSize)
				} else {
					count = len(contentRes.MediaContainerWithMetadata.MediaContainer.Metadata)
				}
			}

			return &LibraryInfo{
				ID:    key,
				Title: title,
				Type:  string(dir.Type),
				Count: count,
			}, nil
		})

		if err != nil {
			slog.Warn("Loader: Failed to load library info", "library", title, "error", err)
			sendUpdate(p, fmt.Sprintf("Failed to load %s: %v", title, err))
			continue
		}

		slog.Debug("Loader: Library loaded", "library", title, "items", libInfo.Count)
		libraries = append(libraries, libInfo)
	}

	slog.Info("Loader: Initialization complete", "libraries", len(libraries))
	sendUpdate(1.0, "Initialization complete!")
	updates <- LoaderResult{
		ServerID:  serverID,
		Libraries: libraries,
	}
}
