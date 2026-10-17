package ui

import (
	"testing"

	"github.com/charmbracelet/x/ansi"
)

func TestEllipsisCountsVisibleCells(t *testing.T) {
	styled := "\x1b[38;2;20;200;200mPLAYING\x1b[0m"
	if got := Ellipsis(styled, 7); got != styled {
		t.Fatalf("color escapes caused premature truncation: %q", got)
	}
	if got := Ellipsis(styled+" 01:23", 10); ansi.StringWidth(got) != 10 || ansi.Strip(got) != "PLAYING..." {
		t.Fatalf("incorrect styled truncation: %q", got)
	}
	if Ellipsis(styled, -1) != "" {
		t.Fatal("negative width should be empty")
	}
}
