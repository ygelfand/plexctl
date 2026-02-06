package search

import (
	"context"
	"fmt"
	"log/slog"
	"sync"
	"time"

	"github.com/LukeHagar/plexgo/models/operations"
	"github.com/ygelfand/plexctl/internal/cache"
	"github.com/ygelfand/plexctl/internal/config"
	"github.com/ygelfand/plexctl/internal/plex"
	"github.com/ygelfand/plexctl/internal/ui"
)

type IndexEntry struct {
	RatingKey string `json:"ratingKey"`
	Title     string `json:"title"`
	Type      string `json:"type"`
	SectionID string `json:"sectionId"`
	Library   string `json:"library"`
	// Additional searchable fields
	OriginalTitle string   `json:"originalTitle,omitempty"`
	Summary       string   `json:"summary,omitempty"`
	Year          int      `json:"year,omitempty"`
	Cast          []string `json:"cast,omitempty"`
	Directors     []string `json:"directors,omitempty"`
}

type SearchIndex struct {
	LastIndexed time.Time    `json:"lastIndexed"`
	Entries     []IndexEntry `json:"entries"`
	mu          sync.RWMutex
}

var (
	indexInstance *SearchIndex
	indexOnce     sync.Once
)

func GetIndex() *SearchIndex {
	indexOnce.Do(func() {
		indexInstance = &SearchIndex{}
		_ = indexInstance.Load()
	})
	return indexInstance
}

func (idx *SearchIndex) Load() error {
	cfg := config.Get()
	cm, err := cache.Get(cfg.CacheDir)
	if err != nil {
		return err
	}

	serverID, _, _ := cfg.GetActiveServer()
	key := fmt.Sprintf("%s/search_index", serverID)

	var data SearchIndex
	if err := cm.Get(key, &data); err == nil {
		idx.mu.Lock()
		idx.LastIndexed = data.LastIndexed
		idx.Entries = data.Entries
		idx.mu.Unlock()
		return nil
	}
	return fmt.Errorf("no index found")
}

func (idx *SearchIndex) Save() error {
	cfg := config.Get()
	cm, err := cache.Get(cfg.CacheDir)
	if err != nil {
		return err
	}

	serverID, _, _ := cfg.GetActiveServer()
	key := fmt.Sprintf("%s/search_index", serverID)

	idx.mu.RLock()
	defer idx.mu.RUnlock()
	return cm.Set(key, idx, 0)
}

type IndexProgress struct {
	Current int
	Total   int
	Message string
}

func (idx *SearchIndex) Reindex(ctx context.Context, progress chan<- IndexProgress) error {
	client, err := plex.NewClient()
	if err != nil {
		return err
	}

	res, err := client.SDK.Library.GetSections(ctx)
	if err != nil {
		return err
	}

	if res.Object == nil || res.Object.MediaContainer == nil {
		return fmt.Errorf("no libraries found")
	}

	var newEntries []IndexEntry
	dirs := res.Object.MediaContainer.Directory
	totalLibs := len(dirs)

	for i, lib := range dirs {
		if lib.Key == nil || lib.Title == nil {
			continue
		}

		slog.Debug("SearchIndex: indexing library", "title", *lib.Title, "index", i+1, "total", totalLibs)
		progress <- IndexProgress{
			Current: i + 1,
			Total:   totalLibs,
			Message: fmt.Sprintf("Indexing %s...", *lib.Title),
		}

		entries, err := idx.indexLibrary(ctx, client, *lib.Key, *lib.Title, i+1, totalLibs, progress)
		if err != nil {
			slog.Error("SearchIndex: failed to index library", "library", *lib.Title, "error", err)
			continue
		}
		newEntries = append(newEntries, entries...)
	}

	idx.mu.Lock()
	idx.Entries = newEntries
	idx.LastIndexed = time.Now()
	idx.mu.Unlock()

	slog.Debug("SearchIndex: reindex complete", "total_entries", len(newEntries))
	progress <- IndexProgress{
		Current: totalLibs,
		Total:   totalLibs,
		Message: "Finalizing...",
	}

	return idx.Save()
}

func (idx *SearchIndex) indexLibrary(ctx context.Context, client *plex.Client, sectionID, libTitle string, libIdx, libTotal int, progress chan<- IndexProgress) ([]IndexEntry, error) {
	var entries []IndexEntry
	start := 0
	size := 100

	for {
		slog.Log(context.Background(), config.LevelTrace, "SearchIndex: fetching page", "library", libTitle, "start", start, "size", size)
		req := operations.ListContentRequest{
			SectionID:           sectionID,
			XPlexContainerStart: ui.Ptr(start),
			XPlexContainerSize:  ui.Ptr(size),
		}
		res, err := client.SDK.Content.ListContent(ctx, req)
		if err != nil {
			return entries, err
		}

		if res.MediaContainerWithMetadata == nil || res.MediaContainerWithMetadata.MediaContainer == nil {
			break
		}

		mc := res.MediaContainerWithMetadata.MediaContainer
		metadata := mc.Metadata
		if len(metadata) == 0 {
			break
		}

		totalInLib := 0
		if mc.TotalSize != nil {
			totalInLib = int(*mc.TotalSize)
		} else {
			totalInLib = len(metadata) // fallback
		}

		for j, meta := range metadata {
			currentPos := start + j + 1
			progress <- IndexProgress{
				Current: libIdx,
				Total:   libTotal,
				Message: fmt.Sprintf("%s: %d/%d", libTitle, currentPos, totalInLib),
			}

			entry := IndexEntry{
				RatingKey: ui.PtrToString(meta.RatingKey),
				Title:     meta.Title,
				Type:      meta.Type,
				SectionID: sectionID,
				Library:   libTitle,
				Summary:   ui.PtrToString(meta.Summary),
			}
			if meta.OriginalTitle != nil {
				entry.OriginalTitle = *meta.OriginalTitle
			}
			if meta.Year != nil {
				entry.Year = int(*meta.Year)
			}

			for _, r := range meta.Role {
				entry.Cast = append(entry.Cast, r.Tag)
			}
			for _, d := range meta.Director {
				entry.Directors = append(entry.Directors, d.Tag)
			}

			entries = append(entries, entry)

			if meta.Type == "show" && meta.RatingKey != nil {
				seasons, _ := plex.GetChildren(ctx, *meta.RatingKey)
				for _, season := range seasons {
					entries = append(entries, IndexEntry{
						RatingKey: ui.PtrToString(season.RatingKey),
						Title:     fmt.Sprintf("%s - %s", meta.Title, season.Title),
						Type:      "season",
						SectionID: sectionID,
						Library:   libTitle,
					})

					episodes, _ := plex.GetChildren(ctx, *season.RatingKey)
					for _, ep := range episodes {
						epEntry := IndexEntry{
							RatingKey: ui.PtrToString(ep.RatingKey),
							Title:     fmt.Sprintf("%s - %s", meta.Title, ep.Title),
							Type:      "episode",
							SectionID: sectionID,
							Library:   libTitle,
							Summary:   ui.PtrToString(ep.Summary),
						}
						if ep.Year != nil {
							epEntry.Year = int(*ep.Year)
						}
						entries = append(entries, epEntry)
					}
				}
			}
		}

		start += size
		if mc.TotalSize != nil && int64(start) >= *mc.TotalSize {
			break
		}
	}

	return entries, nil
}
