package detail

import (
	"errors"
	"strings"
	"testing"

	"github.com/LukeHagar/plexgo/models/components"
	"github.com/ygelfand/plexctl/internal/config"
	"github.com/ygelfand/plexctl/internal/ui"
)

func TestDetailResponsesStayWithServerAndItem(t *testing.T) {
	server := config.ServerTarget{ID: "friend", Token: "shared", Shared: true}
	v := NewMovieDetailView("1", ui.PlexctlTheme, server)
	v.Update(DataMsg{serverID: "different-server", ratingKey: "1", metadata: &components.Metadata{Title: "Wrong movie"}})
	v.Update(DataMsg{serverID: server.CacheNamespace(), ratingKey: "2", metadata: &components.Metadata{Title: "Another movie"}})
	if v.Metadata != nil || !v.Loading {
		t.Fatal("a delayed response populated the wrong detail view")
	}
	v.Update(v.dataError(errors.New("server offline")))
	if !strings.Contains(v.View(), "server offline") || !strings.Contains(v.View(), "Retry") {
		t.Fatal("shared detail failure did not offer retry")
	}
	manager := DetailManager{View: v}
	cmd, handled := manager.Update(ui.KeyPress("r"))
	if !handled || cmd == nil || !v.Loading {
		t.Fatal("retry did not start a new detail request")
	}
	v.Update(DataMsg{serverID: server.CacheNamespace(), ratingKey: "1", metadata: &components.Metadata{Title: "Shared movie"}})
	if v.Err != nil || v.Metadata == nil || v.Metadata.Title != "Shared movie" {
		t.Fatal("successful retry did not clear the error")
	}
}
