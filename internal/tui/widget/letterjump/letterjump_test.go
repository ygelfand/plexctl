package letterjump

import (
	"testing"

	"github.com/ygelfand/plexctl/internal/ui"
)

func TestPlaybackAndQuitKeysRemainLetterDestinations(t *testing.T) {
	for _, key := range []string{"p", "q", "m", "z", "#"} {
		m := New(ui.PlexctlTheme)
		model, cmd := m.Update(ui.KeyPress(key))
		if model != nil || cmd == nil {
			t.Fatalf("%s did not choose a letter", key)
		}
		if _, ok := cmd().(ui.JumpToLetterMsg); !ok {
			t.Fatalf("%s activated a non-navigation action", key)
		}
	}
}
