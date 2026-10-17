package cache

import (
	"bytes"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sync"
	"testing"
	"time"
)

func TestGetConcurrentAndSeparateDirectories(t *testing.T) {
	dir := t.TempDir()
	const workers = 24
	managers := make([]*Manager, workers)
	errs := make(chan error, workers)
	var wg sync.WaitGroup
	for i := range workers {
		wg.Go(func() {
			manager, err := Get(filepath.Join(dir, fmt.Sprint(i%2)))
			managers[i] = manager
			errs <- err
		})
	}
	wg.Wait()
	close(errs)
	for err := range errs {
		if err != nil {
			t.Fatal(err)
		}
	}
	for i, manager := range managers {
		if manager != managers[i%2] {
			t.Fatal("concurrent cache initialization returned different managers for one directory")
		}
	}
	if err := managers[0].Set("same-key", "first directory", time.Hour); err != nil {
		t.Fatal(err)
	}
	var value string
	if err := managers[1].Get("same-key", &value); err == nil {
		t.Fatalf("different cache directory exposed %q", value)
	}
}

func TestBinaryRoundTrip(t *testing.T) {
	m, err := Get(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	for _, tt := range []struct {
		name string
		data []byte
	}{
		{"image", []byte{0x89, 'P', 'N', 'G', '\r', '\n', 0x1a, '\n', 0, 0xff}},
		{"JSON-looking bytes", []byte(`{"image":"bytes"}`)},
		{"empty", []byte{}},
		{"nil", nil},
	} {
		t.Run(tt.name, func(t *testing.T) {
			if err := m.Set(tt.name, tt.data, time.Hour); err != nil {
				t.Fatalf("cache binary value: %v", err)
			}
			var got []byte
			if err := m.Get(tt.name, &got); err != nil {
				t.Fatal(err)
			}
			if !bytes.Equal(got, tt.data) {
				t.Fatalf("binary value changed: %x != %x", got, tt.data)
			}
		})
	}
}

func TestWithCachePreservesFetchedValueOnWriteFailure(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "cache")
	m, err := Get(dir)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.Remove(dir); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(dir, []byte("not a directory"), 0o600); err != nil {
		t.Fatal(err)
	}
	want := "successful network response"
	var got string
	if err := WithCache(m, "key", time.Hour, &got, func() (*string, error) { return &want, nil }); err != nil {
		t.Fatalf("cache failure replaced successful response: %v", err)
	}
	if got != want {
		t.Fatalf("got %q, want %q", got, want)
	}
	if err := WithCache[string](nil, "key", time.Hour, &got, func() (*string, error) { return &want, nil }); err != nil {
		t.Fatal(err)
	}
	fetchErr := errors.New("fetch failed")
	if err := WithCache[string](nil, "key", time.Hour, &got, func() (*string, error) { return nil, fetchErr }); !errors.Is(err, fetchErr) {
		t.Fatalf("fetch error was hidden: %v", err)
	}
}
