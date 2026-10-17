package player

import (
	"encoding/json"
	"fmt"
	"log/slog"
	"time"
)

// loadWindowMedia uses named IPC arguments: mpv 0.38 inserted an argument before
// options in loadfile's positional form. Named arguments work on either side of
// that change and preserve per-file resume offsets without version guessing.
func loadWindowMedia(socket, url string, startOffset int64) error {
	conn, err := dialTerminal(socket)
	if err != nil {
		return fmt.Errorf("connect to mpv for playback: %w", err)
	}
	defer func() {
		if err := conn.Close(); err != nil {
			slog.Debug("Close playback command connection", "error", err)
		}
	}()
	if err := conn.SetDeadline(time.Now().Add(5 * time.Second)); err != nil {
		return err
	}
	request := map[string]any{
		"request_id": 1,
		"command":    mediaLoadCommand(url, startOffset, false),
	}
	if err := json.NewEncoder(conn).Encode(request); err != nil {
		return fmt.Errorf("send playback command: %w", err)
	}
	decoder := json.NewDecoder(conn)
	for {
		var response struct {
			RequestID int    `json:"request_id"`
			Error     string `json:"error"`
		}
		if err := decoder.Decode(&response); err != nil {
			return fmt.Errorf("read playback response: %w", err)
		}
		if response.RequestID != 1 {
			continue
		}
		if response.Error != "success" {
			return fmt.Errorf("mpv could not start playback: %s", response.Error)
		}
		return nil
	}
}

func mediaLoadCommand(url string, startOffset int64, audioOnly bool) map[string]any {
	options := map[string]string{
		"start": fmt.Sprintf("%.3f", float64(startOffset)/1000),
		"vid":   "auto", "force-window": "yes",
	}
	if audioOnly {
		options["vid"] = "no"
		options["force-window"] = "no"
		options["vo"] = "null"
	}
	return map[string]any{"name": "loadfile", "url": url, "flags": "replace", "options": options}
}
