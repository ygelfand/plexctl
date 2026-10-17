//go:build !windows

package player

import (
	"encoding/json"
	"fmt"
	"net"
	"os"
	"path/filepath"
	"testing"
)

func TestWindowLoadNamedArguments(t *testing.T) {
	for _, result := range []string{"success", "invalid parameter"} {
		t.Run(result, func(t *testing.T) {
			dir, err := os.MkdirTemp("", "plexctl-load-")
			if err != nil {
				t.Fatal(err)
			}
			t.Cleanup(func() {
				if err := os.RemoveAll(dir); err != nil {
					t.Error(err)
				}
			})
			path := filepath.Join(dir, "ipc")
			listener, err := net.Listen("unix", path)
			if err != nil {
				t.Fatal(err)
			}
			t.Cleanup(func() {
				if err := listener.Close(); err != nil {
					t.Error(err)
				}
			})
			done := make(chan error, 1)
			go func() {
				defer close(done)
				conn, err := listener.Accept()
				if err != nil {
					done <- err
					return
				}
				defer func() {
					if err := conn.Close(); err != nil {
						done <- err
					}
				}()
				var request struct {
					Command map[string]any `json:"command"`
				}
				if err := json.NewDecoder(conn).Decode(&request); err != nil {
					done <- err
					return
				}
				cmd := request.Command
				options, ok := cmd["options"].(map[string]any)
				if !ok || cmd["name"] != "loadfile" || cmd["flags"] != "replace" || cmd["url"] != "https://plex.invalid/music.flac" || options["start"] != "12.345" {
					done <- fmt.Errorf("incorrect named loadfile request: %+v", cmd)
					return
				}
				encoder := json.NewEncoder(conn)
				if err := encoder.Encode(map[string]any{"event": "start-file"}); err != nil {
					done <- err
					return
				}
				done <- encoder.Encode(map[string]any{"request_id": 1, "error": result})
			}()
			if err := loadWindowMedia(path, "https://plex.invalid/music.flac", 12345); (err != nil) != (result != "success") {
				t.Errorf("loadWindowMedia returned %v for %s", err, result)
			}
			for err := range done {
				if err != nil {
					t.Error(err)
				}
			}
		})
	}
}
