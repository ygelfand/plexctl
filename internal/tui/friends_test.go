package tui

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/ygelfand/plexctl/internal/config"
	"github.com/ygelfand/plexctl/internal/plex"
	"github.com/ygelfand/plexctl/internal/tui/view"
	"github.com/ygelfand/plexctl/internal/tui/widget/poster"
	"github.com/ygelfand/plexctl/internal/ui"
)

func TestFriendsSidebarAndPageRouting(t *testing.T) {
	cfg := config.Get()
	old := *cfg
	t.Cleanup(func() { *cfg = old })
	cfg.DefaultServer, cfg.Token, cfg.DefaultViewMode = "mine", "owned", config.ViewModeList
	cfg.HomeUser = config.HomeUser{}
	cfg.CacheDir, cfg.NoCache = t.TempDir(), true
	cfg.Servers = map[string]config.Server{"mine": {Name: "Mine"}}
	ui.GetLayout().Update(120, 40, false)
	tm := NewTabManager(plex.LoaderResult{Libraries: []plex.LibraryInfo{{ID: "1", Title: "Own Movies"}}}, ui.PlexctlTheme, ui.SidebarWidth)
	tm.navbar.Height = 40
	if !strings.Contains(strings.ToLower(tm.navbar.View()), "friends libraries") {
		t.Fatal("missing Friends Libraries section")
	}
	friends := tm.tabModels[tm.friendsIdx].(*view.FriendsView)
	target := config.ServerTarget{ID: "friend", Name: "Friend", Token: "shared", Shared: true}
	friends.Update(view.FriendsLibrariesMsg{Server: target, Libraries: []plex.LibraryInfo{{ID: "1", Title: "Shared Movies"}}})
	friends.Update(ui.KeyPress("enter")) // Start a library request without making a network call.
	// The global page request IDs are intentionally not assumed here. This
	// verifies late artwork and target selection while switching active tabs.
	c := &Controller{tabManager: tm, navigator: NewNavigator(ui.PlexctlTheme), theme: ui.PlexctlTheme}
	c.tabManager.SetActive(tm.friendsIdx)
	if c.mediaServer().ID != "friend" {
		t.Fatal("friend selection lost its server")
	}
	c.tabManager.SetActive(1)
	if c.mediaServer().ID != "mine" {
		t.Fatal("owned tab kept the friend's server")
	}
	c.Update(poster.PosterLoadedMsg{ServerID: target.CacheNamespace(), RatingKey: "1", View: "friend artwork"})
	if cfg.DefaultServer != "mine" {
		t.Fatal("friends changed default_server")
	}

	endpoint := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("X-Plex-Token") != "shared" {
			t.Error("friend page used wrong credentials")
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"MediaContainer":{"totalSize":1,"Metadata":[{"ratingKey":"1","title":"Shared Film","type":"movie"}]}}`))
	}))
	defer endpoint.Close()
	target.URL = endpoint.URL
	friendView := view.NewMediaView("1", "Friend Movies", ui.PlexctlTheme, target)
	owned := view.NewMediaView("1", "Own Movies", ui.PlexctlTheme, cfg.ServerTarget())
	tm.tabModels, tm.activeTabIdx = []tea.Model{owned, friendView}, 0
	owned.Init()
	page := friendView.Init()()
	c.Update(page)
	if owned.GetSelectedMetadata() != nil {
		t.Fatal("friend response populated the owned library")
	}
	if selected := friendView.GetSelectedMetadata(); selected == nil || selected.Title != "Shared Film" {
		t.Fatal("late page was not routed to its friend's view")
	}
}
