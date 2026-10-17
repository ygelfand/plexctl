package mpvsetup

import (
	"context"
	"errors"
	"fmt"
	"slices"
	"strings"
	"testing"
)

func TestAPTPackageVersions(t *testing.T) {
	query := func(_ context.Context, name string, args ...string) ([]byte, error) {
		if name != "apt-cache" || !slices.Equal(args, []string{"policy", "mpv", "meson", "libplacebo-dev"}) {
			t.Fatalf("unexpected query: %s %v", name, args)
		}
		return []byte(`mpv:
  Installed: 0.37.0-1ubuntu4
  Candidate: 1:0.41.0-2
  Version table:
     1:0.41.0-2 500
        500 http://example.invalid noble/universe arm64 Packages
meson:
  Installed: (none)
  Candidate: 1.3.2-1ubuntu1
libplacebo-dev:arm64:
  Installed: (none)
  Candidate: (none)
`), nil
	}
	got, err := aptPackages(t.Context(), query, []string{"mpv", "meson", "libplacebo-dev"})
	if err != nil {
		t.Fatal(err)
	}
	if got["mpv"] != (aptPackage{"0.37.0-1ubuntu4", "1:0.41.0-2"}) ||
		got["meson"] != (aptPackage{"", "1.3.2-1ubuntu1"}) || got["libplacebo-dev"] != (aptPackage{}) {
		t.Fatalf("parsed package versions: %#v", got)
	}
	_, err = aptPackages(t.Context(), func(context.Context, string, ...string) ([]byte, error) {
		return nil, errors.New("query failed")
	}, nil)
	if err == nil {
		t.Fatal("failed apt query must not look like missing packages")
	}
}

func TestOlderAPT(t *testing.T) {
	for _, tt := range []struct {
		value, minimum string
		want           bool
	}{
		{"0.37.0-1ubuntu4", "0.41.0", true},
		{"1:0.41.0-2", "0.41.0", false},
		{"0.41.0~rc1-1", "0.41.0", true},
		{"0.42.0+git123-1", "0.41.0", false},
		{"7:5.1.6-1", "6.1.0", true},
		{"7:6.1-1", "6.1.0", false},
		{"7:6.1.1-3ubuntu5+esm15", "6.1.0", false},
		{"1.3.2-1ubuntu1", "1.3.0", false},
		{"(none)", "0.41.0", true},
		{"unexpected", "0.41.0", true},
	} {
		t.Run(tt.value, func(t *testing.T) {
			if got := olderAPT(tt.value, tt.minimum); got != tt.want {
				t.Fatalf("olderAPT(%q, %q) = %v", tt.value, tt.minimum, got)
			}
		})
	}
}

func dependencyPolicy(packages map[string]aptPackage) string {
	var result strings.Builder
	for name, p := range packages {
		fmt.Fprintf(&result, "%s:\n  Installed: %s\n  Candidate: %s\n", name, availableVersion(p.installed), availableVersion(p.candidate))
	}
	return strings.ReplaceAll(result.String(), "(unavailable)", "(none)")
}

func availableDependencies() map[string]aptPackage {
	packages := make(map[string]aptPackage)
	for _, deps := range [][]buildDependency{mpvBuildDependencies, placeboBuildDependencies, {{"libplacebo-dev", placeboVersion}}} {
		for _, dep := range deps {
			packages[dep.name] = aptPackage{"9.99.99", "9.99.99"}
		}
	}
	return packages
}

func TestSourceDependencies(t *testing.T) {
	for _, tt := range []struct {
		name, pkg          string
		state              aptPackage
		missing, wantError bool
		fallback           bool
	}{
		{name: "ready"},
		{name: "meson missing", pkg: "meson", state: aptPackage{"", "1.3.2-1"}, missing: true},
		{name: "meson upgrade", pkg: "meson", state: aptPackage{"0.63.0-1", "1.3.2-1"}, missing: true},
		{name: "meson too old", pkg: "meson", state: aptPackage{"", "0.63.0-1"}, missing: true, wantError: true},
		{name: "old ffmpeg epoch", pkg: "libavcodec-dev", state: aptPackage{"7:5.1.6-1", "7:5.1.6-1"}, missing: true, wantError: true},
		{name: "missing sixel", pkg: "libsixel-dev", state: aptPackage{"", "1.10.3-1"}, missing: true},
		{name: "sixel unavailable", pkg: "libsixel-dev", missing: true, wantError: true},
		{name: "placebo upgrade", pkg: "libplacebo-dev", state: aptPackage{"4.208.0-1", "6.338.2-1"}, missing: true},
		{name: "placebo source", pkg: "libplacebo-dev", state: aptPackage{"4.208.0-1", "4.208.0-1"}, fallback: true},
		{name: "placebo absent", pkg: "libplacebo-dev", fallback: true},
		{name: "installed newer than repo", pkg: "libplacebo-dev", state: aptPackage{"7.349.0-1", "4.208.0-1"}},
	} {
		t.Run(tt.name, func(t *testing.T) {
			packages := availableDependencies()
			if tt.pkg != "" {
				packages[tt.pkg] = tt.state
			}
			plan, err := sourceDependencies(t.Context(), func(context.Context, string, ...string) ([]byte, error) {
				return []byte(dependencyPolicy(packages)), nil
			})
			if (err != nil) != tt.wantError || plan.placebo != tt.fallback || slices.Contains(plan.missing, tt.pkg) != tt.missing {
				t.Fatalf("plan=%+v err=%v", plan, err)
			}
			if err != nil && !strings.Contains(err.Error(), tt.pkg) {
				t.Fatalf("preflight error must identify dependency: %v", err)
			}
		})
	}
}
