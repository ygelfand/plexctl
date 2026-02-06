package plex

import (
	"context"
	"fmt"
	"sync"

	"github.com/LukeHagar/plexgo/models/components"
	"github.com/LukeHagar/plexgo/models/operations"
	"github.com/ygelfand/plexctl/internal/cache"
	"github.com/ygelfand/plexctl/internal/config"
)

type Store struct {
	client *Client
	cache  *cache.Manager

	// Cache for IDs to Titles
	userCache    map[int64]string
	deviceCache  map[string]string
	libraryCache map[string]string
	mu           sync.RWMutex
}

var (
	storeInstance *Store
	storeOnce     sync.Once
)

func GetStore() (*Store, error) {
	var err error
	storeOnce.Do(func() {
		client, cErr := NewClient()
		if cErr != nil {
			err = cErr
			return
		}

		cfg := config.Get()
		cm, cErr := cache.Get(cfg.CacheDir)
		if cErr != nil {
			err = cErr
			return
		}

		storeInstance = &Store{
			client:       client,
			cache:        cm,
			userCache:    make(map[int64]string),
			deviceCache:  make(map[string]string),
			libraryCache: make(map[string]string),
		}
	})
	return storeInstance, err
}

func (s *Store) ResolveUser(ctx context.Context, id int64) (string, error) {
	s.mu.RLock()
	if name, ok := s.userCache[id]; ok {
		s.mu.RUnlock()
		return name, nil
	}
	s.mu.RUnlock()

	// Refresh cache
	res, err := s.client.SDK.Users.GetUsers(ctx, operations.GetUsersRequest{})
	if err != nil {
		return fmt.Sprintf("%d", id), err
	}

	s.mu.Lock()
	defer s.mu.Unlock()
	if res.Object != nil && res.Object.MediaContainer != nil {
		for _, u := range res.Object.MediaContainer.User {
			s.userCache[u.ID] = u.Title
		}
	}

	if name, ok := s.userCache[id]; ok {
		return name, nil
	}
	return fmt.Sprintf("%d", id), nil
}

func (s *Store) ResolveDevice(ctx context.Context, id string) (string, error) {
	s.mu.RLock()
	if name, ok := s.deviceCache[id]; ok {
		s.mu.RUnlock()
		return name, nil
	}
	s.mu.RUnlock()

	res, err := s.client.SDK.Devices.ListDevices(ctx)
	if err != nil {
		return id, err
	}

	s.mu.Lock()
	defer s.mu.Unlock()
	if res.MediaContainerWithDevice != nil && res.MediaContainerWithDevice.MediaContainer != nil {
		for _, d := range res.MediaContainerWithDevice.MediaContainer.Device {
			if d.UUID != nil {
				name := "Unknown"
				if d.Model != nil {
					name = *d.Model
				} else if d.Make != nil {
					name = *d.Make
				}
				s.deviceCache[*d.UUID] = name
			}
		}
	}

	if name, ok := s.deviceCache[id]; ok {
		return name, nil
	}
	return id, nil
}

func (s *Store) ResolveLibrary(ctx context.Context, id string) (string, error) {
	s.mu.RLock()
	if name, ok := s.libraryCache[id]; ok {
		s.mu.RUnlock()
		return name, nil
	}
	s.mu.RUnlock()

	res, err := s.client.SDK.Library.GetSections(ctx)
	if err != nil {
		return id, err
	}

	s.mu.Lock()
	defer s.mu.Unlock()
	if res.Object != nil && res.Object.MediaContainer != nil {
		for _, l := range res.Object.MediaContainer.Directory {
			if l.Key != nil && l.Title != nil {
				s.libraryCache[*l.Key] = *l.Title
			}
		}
	}

	if name, ok := s.libraryCache[id]; ok {
		return name, nil
	}
	return id, nil
}

func (s *Store) ListPlaybackHistory(ctx context.Context, request operations.ListPlaybackHistoryRequest, opts ...operations.Option) (*operations.ListPlaybackHistoryResponse, error) {
	return s.client.SDK.Status.ListPlaybackHistory(ctx, request, opts...)
}

func (s *Store) GetMetadata(ctx context.Context, ratingKey string, force bool) (*components.Metadata, error) {
	cfg := config.Get()
	serverID, _, _ := cfg.GetActiveServer()

	var body components.MediaContainerWithMetadata
	req := operations.GetMetadataItemRequest{
		Ids:           []string{ratingKey},
		IncludeExtras: components.BoolIntTrue.ToPointer(),
	}

	ttl := MediaCacheTTL
	if force {
		ttl = 0
	}

	err := cache.AutoCache(s.cache, serverID, req, ttl, &body, func() (*components.MediaContainerWithMetadata, error) {
		res, err := s.client.SDK.Content.GetMetadataItem(ctx, req)
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

	if len(body.MediaContainer.Metadata) == 0 {
		return nil, fmt.Errorf("metadata empty")
	}

	return &body.MediaContainer.Metadata[0], nil
}
