package presenters

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/LukeHagar/plexgo/models/components"
)

func TestDeviceAndServerOutputOmitTokens(t *testing.T) {
	devices := []components.PlexDevice{{Name: "server", AccessToken: "secret-resource-token"}}
	for _, tt := range []struct {
		name string
		raw  any
	}{
		{"servers", (&ServerListPresenter{Devices: devices}).Raw()},
		{"devices", (&DevicesPresenter{Devices: devices}).Raw()},
	} {
		t.Run(tt.name, func(t *testing.T) {
			data, err := json.Marshal(tt.raw)
			if err != nil {
				t.Fatal(err)
			}
			if strings.Contains(string(data), "secret-resource-token") {
				t.Fatal("machine-readable device output exposes a resource token")
			}
			if devices[0].AccessToken != "secret-resource-token" {
				t.Fatal("rendering output changed the caller's credentials")
			}
		})
	}
}
