package search

import (
	"context"
	"fmt"
	"log/slog"
	"slices"
	"strings"
	"sync"
	"time"

	"github.com/LukeHagar/plexgo/models/components"
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
	Artist    string `json:"artist,omitempty"`
	Album     string `json:"album,omitempty"`
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
	reindexMu   sync.Mutex
	scopeOnce   sync.Once
	server      config.ServerTarget
	cacheDir    string
}

// SearchableText includes music context, so an album or track can also be found
// by artist. Titles remain first for the fuzzy matcher's ranking.
func (e IndexEntry) SearchableText() string {
	return strings.Join([]string{e.Title, e.Artist, e.Album, e.OriginalTitle,
		strings.Join(e.Cast, " "), strings.Join(e.Directors, " ")}, " ")
}

func (e IndexEntry) Description() string {
	parts := []string{strings.ToUpper(e.Type)}
	for _, part := range []string{e.Artist, e.Album, e.Library} {
		if part != "" && part != e.Title {
			parts = append(parts, part)
		}
	}
	return strings.Join(parts, " · ")
}

// Snapshot returns an immutable view of the last completed index.
func (idx *SearchIndex) Snapshot() ([]IndexEntry, time.Time) {
	idx.mu.RLock()
	defer idx.mu.RUnlock()
	return slices.Clone(idx.Entries), idx.LastIndexed
}

var (
	indexInstance  *SearchIndex
	indexMu        sync.Mutex
	indexNamespace string
)

func GetIndex() *SearchIndex {
	indexMu.Lock()
	defer indexMu.Unlock()
	cfg := config.Get()
	namespace := cfg.MediaCacheNamespace()
	if indexInstance == nil || indexNamespace != namespace || indexInstance.cacheDir != cfg.CacheDir {
		indexInstance = &SearchIndex{}
		indexInstance.bindScope()
		indexNamespace = namespace
		_ = indexInstance.Load()
	}
	return indexInstance
}

func (idx *SearchIndex) Load() error {
	idx.bindScope()
	cm, err := cache.Get(idx.cacheDir)
	if err != nil {
		return err
	}

	key := fmt.Sprintf("%s/search_index", idx.server.CacheNamespace())

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
	idx.bindScope()
	cm, err := cache.Get(idx.cacheDir)
	if err != nil {
		return err
	}

	key := fmt.Sprintf("%s/search_index", idx.server.CacheNamespace())

	idx.mu.RLock()
	defer idx.mu.RUnlock()
	return cm.Set(key, idx, 0)
}

// An index retains the identity it was created for, including when a Home
// profile switch happens while a background reindex is still running.
func (idx *SearchIndex) bindScope() {
	idx.scopeOnce.Do(func() {
		cfg := config.Get()
		idx.server, idx.cacheDir = cfg.ServerTarget(), cfg.CacheDir
	})
}

type IndexProgress struct {
	Current int
	Total   int
	Message string
}

func (idx *SearchIndex) Reindex(ctx context.Context, progress chan<- IndexProgress) error {
	idx.bindScope()
	idx.reindexMu.Lock()
	defer idx.reindexMu.Unlock()
	ctx = idx.server.Context(ctx)
	client, err := plex.NewClientForContext(ctx)
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
		notifyProgress(progress, IndexProgress{
			Current: i + 1,
			Total:   totalLibs,
			Message: fmt.Sprintf("Indexing %s...", *lib.Title),
		})

		entries, err := idx.indexLibrary(ctx, client, *lib.Key, *lib.Title, i+1, totalLibs, progress)
		if err != nil {
			return fmt.Errorf("index %s: %w", *lib.Title, err)
		}
		newEntries = append(newEntries, entries...)
	}

	idx.mu.Lock()
	idx.Entries = newEntries
	idx.LastIndexed = time.Now()
	idx.mu.Unlock()

	slog.Debug("SearchIndex: reindex complete", "total_entries", len(newEntries))
	notifyProgress(progress, IndexProgress{
		Current: totalLibs,
		Total:   totalLibs,
		Message: "Finalizing...",
	})

	return idx.Save()
}

func (idx *SearchIndex) indexLibrary(ctx context.Context, client *plex.Client, sectionID, libTitle string, libIdx, libTotal int, progress chan<- IndexProgress) ([]IndexEntry, error) {
	var entries []IndexEntry
	start := 0
	size := 100
	music := false

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
			return nil, fmt.Errorf("missing library metadata")
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
			notifyProgress(progress, IndexProgress{
				Current: libIdx,
				Total:   libTotal,
				Message: fmt.Sprintf("%s: %d/%d", libTitle, currentPos, totalInLib),
			})

			entries = append(entries, entryFromMetadata(meta, sectionID, libTitle))
			music = music || meta.Type == "artist"

			if meta.Type == "show" && meta.RatingKey != nil {
				seasons, err := plex.GetChildren(ctx, *meta.RatingKey)
				if err != nil {
					return nil, err
				}
				for _, season := range seasons {
					if season.RatingKey == nil || *season.RatingKey == "" {
						return nil, fmt.Errorf("season in show %s has no rating key", *meta.RatingKey)
					}
					entries = append(entries, IndexEntry{
						RatingKey: ui.PtrToString(season.RatingKey),
						Title:     fmt.Sprintf("%s - %s", meta.Title, season.Title),
						Type:      "season",
						SectionID: sectionID,
						Library:   libTitle,
					})

					episodes, err := plex.GetChildren(ctx, *season.RatingKey)
					if err != nil {
						return nil, err
					}
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

		start += len(metadata)
		if mc.TotalSize != nil && int64(start) >= *mc.TotalSize {
			break
		}
	}

	if music {
		// Plex's actual library type IDs are album=9 and track=10. The pinned
		// SDK's enum uses incorrect music IDs; explicitly use the wire values.
		for _, mediaType := range []components.MediaType{9, 10} {
			for start := 0; ; {
				res, err := client.SDK.Content.ListContent(ctx, operations.ListContentRequest{
					SectionID: sectionID, XPlexContainerStart: ui.Ptr(start), XPlexContainerSize: ui.Ptr(size),
					MediaQuery: &components.MediaQuery{Type: &mediaType},
				})
				if err != nil {
					return nil, err
				}
				if res.MediaContainerWithMetadata == nil || res.MediaContainerWithMetadata.MediaContainer == nil {
					return nil, fmt.Errorf("missing music metadata")
				}
				mc := res.MediaContainerWithMetadata.MediaContainer
				if len(mc.Metadata) == 0 {
					break
				}
				for _, meta := range mc.Metadata {
					entries = append(entries, entryFromMetadata(meta, sectionID, libTitle))
				}
				start += len(mc.Metadata)
				notifyProgress(progress, IndexProgress{Current: libIdx, Total: libTotal,
					Message: fmt.Sprintf("%s: %d music entries", libTitle, len(entries))})
				if mc.TotalSize != nil && int64(start) >= *mc.TotalSize {
					break
				}
			}
		}
	}

	return entries, nil
}

func notifyProgress(progress chan<- IndexProgress, update IndexProgress) {
	// A hidden status tab must not block indexing. Progress is informational.
	select {
	case progress <- update:
	default:
	}
}

func entryFromMetadata(meta components.Metadata, sectionID, library string) IndexEntry {
	entry := IndexEntry{
		RatingKey: ui.PtrToString(meta.RatingKey), Title: meta.Title, Type: meta.Type,
		SectionID: sectionID, Library: library, Summary: ui.PtrToString(meta.Summary),
		OriginalTitle: ui.PtrToString(meta.OriginalTitle),
	}
	if meta.Year != nil {
		entry.Year = int(*meta.Year)
	}
	switch meta.Type {
	case "album":
		entry.Artist = ui.PtrToString(meta.ParentTitle)
	case "track":
		entry.Artist = ui.PtrToString(meta.GrandparentTitle)
		entry.Album = ui.PtrToString(meta.ParentTitle)
	}
	for _, role := range meta.Role {
		entry.Cast = append(entry.Cast, role.Tag)
	}
	for _, director := range meta.Director {
		entry.Directors = append(entry.Directors, director.Tag)
	}
	return entry
}
