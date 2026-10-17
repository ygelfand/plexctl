package view

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/LukeHagar/plexgo/models/components"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/ygelfand/plexctl/internal/config"
	"github.com/ygelfand/plexctl/internal/search"
	"github.com/ygelfand/plexctl/internal/ui"
)

func TestFriendsBrowseSearchAndBackKeepOwnedServer(t *testing.T) {
	t.Setenv("PLEXCTL_TOKEN", "")
	cfg := config.Get()
	old := *cfg
	t.Cleanup(func() { *cfg = old })
	cfg.Token, cfg.HomeUser = "owned-token", config.HomeUser{}
	cfg.DefaultServer, cfg.CacheDir, cfg.NoCache = "owned", t.TempDir(), true
	cfg.Servers = map[string]config.Server{"owned": {URL: "https://owned.invalid"}}
	cfg.DefaultViewMode = config.ViewModeList
	ui.GetLayout().Update(120, 40, false)
	friend := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		wantToken := "friend-token"
		if r.URL.Path == "/identity" {
			wantToken = ""
		}
		if r.Header.Get("X-Plex-Token") != wantToken {
			t.Error("used owned credentials for a friend")
			http.Error(w, "denied", http.StatusUnauthorized)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		meta := map[string]any{"ratingKey": "1", "title": "Shared Film", "type": "movie", "librarySectionID": "1"}
		var container any
		switch r.URL.Path {
		case "/identity":
			container = map[string]any{"machineIdentifier": "friend"}
		case "/library/sections", "/library/sections/all":
			container = map[string]any{"Directory": []any{map[string]any{"key": "1", "title": "Shared Movies", "type": "movie"}}}
		case "/library/sections/1/all", "/library/metadata/1":
			container = map[string]any{"Metadata": []any{meta}, "totalSize": 1}
		case "/hubs/search":
			container = map[string]any{"Hub": []any{map[string]any{"type": "movie", "Metadata": []any{meta}}}}
		default:
			t.Errorf("unexpected request %s", r.URL.Path)
			http.NotFound(w, r)
			return
		}
		_ = json.NewEncoder(w).Encode(map[string]any{"MediaContainer": container})
	}))
	defer friend.Close()
	v := NewFriendsView(ui.PlexctlTheme)
	v.Update(tea.WindowSizeMsg{Width: 120, Height: 40})
	v.Update(FriendsServersMsg{Servers: []components.PlexDevice{{Name: "Friend", ClientIdentifier: "friend", AccessToken: "friend-token", Connections: []components.Connections{{URI: friend.URL}}}}})
	_, connect := v.Update(ui.KeyPress("enter"))
	v.Update(connect())
	if v.server.ID != "friend" || !strings.Contains(v.View(), "Shared Movies") {
		t.Fatalf("shared libraries were not listed: %s", v.View())
	}
	entries, err := search.Live(v.server.Context(context.Background()), "Shared")
	if err != nil || len(entries) != 1 || entries[0].Title != "Shared Film" {
		t.Fatalf("shared search failed: %v", err)
	}
	_, browse := v.Update(ui.KeyPress("enter"))
	v.Update(browse())
	if selected := v.GetSelectedMetadata(); selected == nil || selected.Title != "Shared Film" {
		t.Fatal("shared media did not load")
	}
	_, details := v.Update(ui.KeyPress("enter"))
	v.Update(details())
	if selected := v.GetSelectedMetadata(); selected == nil || selected.Title != "Shared Film" {
		t.Fatal("shared detail did not load")
	}
	_, back := v.Update(ui.KeyPress("esc"))
	v.Update(back())
	v.Update(ui.KeyPress("esc"))
	if v.media != nil || v.server.ID != "friend" {
		t.Fatal("back did not restore shared libraries")
	}
	v.Update(ui.KeyPress("esc"))
	if v.server.ID != "" || !strings.Contains(v.View(), "Friend") {
		t.Fatal("back did not restore friends")
	}
	if cfg.DefaultServer != "owned" || cfg.ServerToken() != "owned-token" {
		t.Fatal("browsing a friend changed the configured server")
	}
}

func TestFriendsEmptyOfflineAndStaleConnection(t *testing.T) {
	v := NewFriendsView(ui.PlexctlTheme)
	v.Update(FriendsServersMsg{})
	if !strings.Contains(v.View(), "No shared servers") {
		t.Fatal("missing empty-state explanation")
	}
	v.requestID = 42
	v.Update(ui.KeyPress("esc"))
	v.Update(FriendsLibrariesMsg{RequestID: 42, Server: config.ServerTarget{ID: "stale"}})
	if v.server.ID != "" {
		t.Fatal("cancelled connection replaced the view")
	}
	v.Update(FriendsServersMsg{RequestID: v.requestID, Err: context.DeadlineExceeded})
	if !strings.Contains(v.View(), "Refresh") || v.loading {
		t.Fatal("failed discovery cannot be retried")
	}
}
