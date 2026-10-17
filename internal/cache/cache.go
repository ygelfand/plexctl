package cache

import (
	"context"
	"crypto/md5"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"reflect"
	"sync"
	"time"

	"github.com/peterbourgon/diskv"
	"github.com/ygelfand/plexctl/internal/config"
)

type CacheEntry struct {
	Value     json.RawMessage `json:"value"`
	ExpiresAt int64           `json:"expires_at"` // Unix timestamp, 0 for infinite
}

type Manager struct {
	dv *diskv.Diskv
}

var (
	managersMu sync.Mutex
	managers   = make(map[string]*Manager)
)

// Get returns the global cache manager instance initialized with the given path
func Get(path string) (*Manager, error) {
	path, err := filepath.Abs(path)
	if err != nil {
		return nil, err
	}
	managersMu.Lock()
	defer managersMu.Unlock()
	if manager := managers[path]; manager != nil {
		return manager, nil
	}

	if err := os.MkdirAll(path, 0o700); err != nil {
		return nil, err
	}

	flatTransform := func(s string) []string {
		return []string{}
	}

	dv := diskv.New(diskv.Options{
		BasePath:     path,
		Transform:    flatTransform,
		CacheSizeMax: 1024 * 1024, // 1MB
		PathPerm:     0o700,
		FilePerm:     0o600,
	})

	manager := &Manager{dv: dv}
	managers[path] = manager
	return manager, nil
}

// HashKey converts a potentially unsafe string into a safe MD5 hash for disk storage
func (m *Manager) HashKey(key string) string {
	h := md5.New()
	h.Write([]byte(key))
	return hex.EncodeToString(h.Sum(nil))
}

// GenerateKey creates a unique, safe key based on an operation and its parameters.
// It uses reflection to automatically determine the "op" name from the params type if possible.
func (m *Manager) GenerateKey(namespace string, params any) string {
	op := "default"
	if params != nil {
		t := reflect.TypeOf(params)
		if t.Kind() == reflect.Pointer {
			t = t.Elem()
		}
		op = t.String()
	}
	p, err := json.Marshal(params)
	if err != nil {
		// Fallback to a string representation if JSON marshaling fails
		return m.HashKey(fmt.Sprintf("%s:%s:%#v", namespace, op, params))
	}
	return m.HashKey(fmt.Sprintf("%s:%s:%s", namespace, op, string(p)))
}

// Set stores data in the cache under the given key with a TTL.
func (m *Manager) Set(key string, val any, ttl time.Duration) error {
	if config.Get().NoCache {
		return nil
	}
	safeKey := m.HashKey(key)
	slog.Log(context.Background(), config.LevelTrace, "Cache: SET", "key", key, "safeKey", safeKey, "ttl", ttl)
	// Marshal binary values too: encoding/json stores []byte as base64 instead
	// of treating image bytes as raw JSON in the cache envelope.
	data, err := json.Marshal(val)
	if err != nil {
		return err
	}

	var expiresAt int64
	if ttl > 0 {
		expiresAt = time.Now().Add(ttl).Unix()
	}

	entry := CacheEntry{
		Value:     data,
		ExpiresAt: expiresAt,
	}

	entryData, err := json.Marshal(entry)
	if err != nil {
		return err
	}

	return m.dv.Write(safeKey, entryData)
}

// Get retrieves cached data.
func (m *Manager) Get(key string, val any) error {
	if config.Get().NoCache {
		return fmt.Errorf("caching is disabled")
	}
	safeKey := m.HashKey(key)
	slog.Log(context.Background(), config.LevelTrace, "Cache: GET", "key", key, "safeKey", safeKey)
	entryData, err := m.dv.Read(safeKey)
	if err != nil {
		return err
	}

	var entry CacheEntry
	if err := json.Unmarshal(entryData, &entry); err != nil {
		return err
	}

	if entry.ExpiresAt > 0 && time.Now().Unix() >= entry.ExpiresAt {
		slog.Log(context.Background(), config.LevelTrace, "Cache: EXPIRED", "key", key, "safeKey", safeKey)
		_ = m.dv.Erase(safeKey)
		return fmt.Errorf("cache entry expired")
	}

	return json.Unmarshal(entry.Value, val)
}

// WithCache is a helper that tries to get data from cache first, otherwise calls the fetcher
func WithCache[T any](m *Manager, key string, ttl time.Duration, val *T, fetcher func() (*T, error)) error {
	if m != nil && ttl > 0 {
		if err := m.Get(key, val); err == nil {
			return nil
		}
	}
	fetched, err := fetcher()
	if err != nil {
		return err
	}

	if fetched != nil {
		*val = *fetched
		if m != nil && ttl > 0 {
			if err := m.Set(key, fetched, ttl); err != nil {
				slog.Warn("Cache: failed to save fetched data", "error", err)
			}
		}
	}

	return nil
}

// AutoCache automatically generates a key based on the request parameters type.
func AutoCache[T any](m *Manager, namespace string, req any, ttl time.Duration, val *T, fetcher func() (*T, error)) error {
	key := m.GenerateKey(namespace, req)
	return WithCache(m, key, ttl, val, fetcher)
}

// Delete removes a key from the cache
func (m *Manager) Delete(key string) error {
	if config.Get().NoCache {
		return nil
	}
	return m.dv.Erase(m.HashKey(key))
}
