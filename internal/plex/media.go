package plex

import (
	"context"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	neturl "net/url"
	"time"

	"github.com/LukeHagar/plexgo/models/components"
	"github.com/LukeHagar/plexgo/models/operations"
	"github.com/ygelfand/plexctl/internal/cache"
	"github.com/ygelfand/plexctl/internal/config"
)

var (
	MediaCacheTTL  = 1 * time.Hour
	PosterCacheTTL = 30 * 24 * time.Hour // Cache rendered strings for 30 days
)

// GetCachedPoster tries to retrieve a rendered poster string from cache
func GetCachedPoster(ratingKey string, width int) (string, bool) {
	key := fmt.Sprintf("rendered_poster/%s/%d", ratingKey, width)

	cfg := config.Get()
	cm, err := cache.Get(cfg.CacheDir)
	if err != nil {
		return "", false
	}

	var result string
	if err := cm.Get(key, &result); err == nil {
		slog.Debug("Poster Cache HIT (Disk)", "ratingKey", ratingKey, "width", width)
		return result, true
	}
	slog.Debug("Poster Cache MISS", "ratingKey", ratingKey, "width", width)
	return "", false
}

// SetCachedPoster stores a rendered poster string in cache
func SetCachedPoster(ratingKey string, width int, view string) {
	cfg := config.Get()
	cm, err := cache.Get(cfg.CacheDir)
	if err != nil {
		return
	}

	slog.Debug("Poster Cache SAVE", "ratingKey", ratingKey, "width", width)
	key := fmt.Sprintf("rendered_poster/%s/%d", ratingKey, width)
	_ = cm.Set(key, view, PosterCacheTTL)
}

// GetMetadata retrieves full metadata for an item by its rating key. If force is true, cache is bypassed.
func GetMetadata(ctx context.Context, ratingKey string, force bool) (*components.Metadata, error) {
	cfg := config.Get()
	server := config.ServerFromContext(ctx)
	if server.URL == "" {
		return nil, fmt.Errorf("no active server")
	}

	cm, err := cache.Get(cfg.CacheDir)
	if err != nil {
		return nil, err
	}

	client, err := NewClientForContext(ctx)
	if err != nil {
		return nil, err
	}

	var body components.MediaContainerWithMetadata
	req := operations.GetMetadataItemRequest{
		Ids:           []string{ratingKey},
		IncludeExtras: components.BoolIntTrue.ToPointer(),
	}

	ttl := MediaCacheTTL
	if force {
		ttl = 0
	}

	err = cache.AutoCache(cm, server.CacheNamespace(), req, ttl, &body, func() (*components.MediaContainerWithMetadata, error) {
		res, err := client.SDK.Content.GetMetadataItem(ctx, req)
		if err != nil {
			return nil, err
		}
		if res.MediaContainerWithMetadata == nil {
			return nil, fmt.Errorf("metadata not found")
		}
		return res.MediaContainerWithMetadata, nil
	})
	if err != nil {
		return nil, err
	}

	if body.MediaContainer == nil || len(body.MediaContainer.Metadata) == 0 {
		return nil, fmt.Errorf("metadata empty")
	}

	return &body.MediaContainer.Metadata[0], nil
}

func ptr[T any](v T) *T {
	return &v
}

// GetChildren retrieves children of an item (e.g. seasons of a show, or episodes of a season)
func GetChildren(ctx context.Context, ratingKey string) ([]components.Metadata, error) {
	return metadataListing(ctx, "/library/metadata/"+neturl.PathEscape(ratingKey)+"/children", nil)
}

// GetImage retrieves image data (thumb/poster) and caches it
func GetImage(ctx context.Context, path string) ([]byte, error) {
	cfg := config.Get()
	server := config.ServerFromContext(ctx)
	if server.URL == "" {
		return nil, fmt.Errorf("no active server")
	}

	cm, err := cache.Get(cfg.CacheDir)
	if err != nil {
		return nil, err
	}

	cacheKey := fmt.Sprintf("%s/img/%s", server.CacheNamespace(), path)
	var data []byte
	if err := cm.Get(cacheKey, &data); err == nil {
		return data, nil
	}

	u, err := mediaURL(server, path)
	if err != nil {
		return nil, err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, u.String(), nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("X-Plex-Token", server.Token)
	resp, err := mediaHTTPClient().Do(req)
	if err != nil {
		return nil, err
	}
	defer func() {
		if err := resp.Body.Close(); err != nil {
			slog.Debug("Close image response", "error", err)
		}
	}()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("failed to fetch image: %s", resp.Status)
	}

	const maxImageBytes = 16 << 20
	data, err = io.ReadAll(io.LimitReader(resp.Body, maxImageBytes+1))
	if err != nil {
		return nil, err
	}
	if len(data) > maxImageBytes {
		return nil, fmt.Errorf("plex image exceeds the 16 MiB limit")
	}

	_ = cm.Set(cacheKey, data, MediaCacheTTL)

	return data, nil
}

// GetHomeHubs retrieves all promoted hubs for the home screen
func GetHomeHubs(ctx context.Context) ([]components.Hub, error) {
	client, err := NewClientForContext(ctx)
	if err != nil {
		return nil, err
	}

	res, err := client.SDK.Hubs.GetPromotedHubs(ctx, operations.GetPromotedHubsRequest{
		Count: ptr(int64(50)),
	})
	if err != nil {
		return nil, err
	}

	if res.Object == nil || res.Object.MediaContainer == nil {
		return nil, fmt.Errorf("no hubs found")
	}

	return res.Object.MediaContainer.Hub, nil
}
