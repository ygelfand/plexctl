package player

import (
	"context"
	"encoding/json"
	"fmt"
	"net"
	"time"
)

// ipcClient is owned by PlayerManager's command loop. Every request has a
// deadline: a dead player cannot hang a control, shutdown, or the visualizer.
type ipcClient struct {
	conn    net.Conn
	encoder *json.Encoder
	decoder *json.Decoder
	nextID  uint64
	events  []terminalEvent
}

func openIPC(socket string) (*ipcClient, error) {
	conn, err := dialTerminal(socket)
	if err != nil {
		return nil, err
	}
	return &ipcClient{conn: conn, encoder: json.NewEncoder(conn), decoder: json.NewDecoder(conn)}, nil
}

func (c *ipcClient) command(command any, result any) error {
	c.nextID++
	if err := c.conn.SetDeadline(time.Now().Add(2 * time.Second)); err != nil {
		return err
	}
	if err := c.encoder.Encode(map[string]any{"command": command, "request_id": c.nextID}); err != nil {
		return err
	}
	for {
		var reply struct {
			terminalEvent
			ID    uint64          `json:"request_id"`
			Error string          `json:"error"`
			Data  json.RawMessage `json:"data"`
		}
		if err := c.decoder.Decode(&reply); err != nil {
			return err
		}
		if reply.Event == "start-file" || reply.Event == "end-file" || reply.Event == "file-loaded" {
			c.events = append(c.events, reply.terminalEvent)
		}
		if reply.ID != c.nextID {
			continue
		}
		if reply.Error != "success" {
			return fmt.Errorf("mpv: %s", reply.Error)
		}
		if result != nil && len(reply.Data) > 0 {
			return json.Unmarshal(reply.Data, result)
		}
		return nil
	}
}

// loadfile's acknowledgement precedes decoder/VO initialization. Check the
// actual result while the first item is paused, before declaring success.
func (c *ipcClient) waitInitialFile(ctx context.Context, video bool) error {
	ctx, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()
	ticker := time.NewTicker(20 * time.Millisecond)
	defer ticker.Stop()
	for {
		if err := c.get("idle-active", nil); err != nil {
			return fmt.Errorf("mpv disconnected while opening the stream: %w", err)
		}
		loaded := false
		for _, event := range c.events {
			if event.Event == "end-file" && event.Reason == "error" {
				return fmt.Errorf("mpv could not open the stream or initialize the video output")
			}
			loaded = loaded || event.Event == "file-loaded"
		}
		if loaded {
			configured := !video
			if video {
				_ = c.get("vo-configured", &configured)
			}
			if configured {
				return nil
			}
		}
		select {
		case <-ctx.Done():
			return fmt.Errorf("mpv did not finish opening the stream and video output: %w", ctx.Err())
		case <-ticker.C:
		}
	}
}

func (c *ipcClient) call(args ...any) error {
	return c.command(args, nil)
}

func (c *ipcClient) get(property string, result any) error {
	return c.command([]any{"get_property", property}, result)
}
