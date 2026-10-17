package player

import (
	"slices"
	"strings"
	"testing"
)

func TestMusicVideoGraphs(t *testing.T) {
	for _, tt := range []struct{ name, filter string }{
		{name: "cqt", filter: "showcqt="}, {name: "spectrum", filter: "showspectrum="},
		{name: "waves", filter: "showwaves="}, {name: "scope", filter: "avectorscope="},
	} {
		t.Run(tt.name, func(t *testing.T) {
			graph := musicVideoGraph(tt.name)
			if !strings.HasPrefix(graph, "[aid1]asplit[ao][vis];") || !strings.Contains(graph, tt.filter) {
				t.Fatalf("visualizer changes audible routing or selects wrong graph: %s", graph)
			}
		})
	}
	if musicVideoGraph("off") != "" {
		t.Fatal("disabled visualizer still creates video")
	}
}

func TestSixelSizeCapAndCentering(t *testing.T) {
	args := sixelSizeArgs(640, 200, 50, 10, 20)
	for _, want := range []string{"--vo-sixel-width=640", "--vo-sixel-cols=64", "--vo-sixel-left=69", "--vo-sixel-height=1000"} {
		if !slices.Contains(args, want) {
			t.Fatalf("missing %s: %v", want, args)
		}
	}
	if args := sixelSizeArgs(0, 200, 50, 10, 20); !slices.Contains(args, "--vo-sixel-width=2000") {
		t.Fatalf("uncapped playback must still fit the pane: %v", args)
	}
	if args := sixelSizeArgs(640, 40, 25, 8, 16); !slices.Contains(args, "--vo-sixel-width=320") {
		t.Fatalf("sizing exceeds a narrow terminal: %v", args)
	}
}
