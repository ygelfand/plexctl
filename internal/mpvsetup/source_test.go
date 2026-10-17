package mpvsetup

import (
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"slices"
	"strings"
	"testing"
)

func TestSourcePlan(t *testing.T) {
	root := t.TempDir()
	missing := func(string) (string, error) { return "", exec.ErrNotFound }
	deps := dependencyPlan{missing: []string{"meson", "libsixel-dev"}, placebo: true}
	if _, err := newSourcePlan("0.41.0", root, root, deps, false, missing); err == nil {
		t.Fatal("dependency installation requires privilege")
	}
	p, err := newSourcePlan("0.41.0", root, root, deps, false, func(string) (string, error) { return "/bin/sudo", nil })
	if err != nil {
		t.Fatal(err)
	}
	if !slices.Equal(p.packages.Commands[1], []string{"sudo", "apt-get", "install", "--yes", "--no-remove", "meson", "libsixel-dev"}) {
		t.Fatalf("unexpected dependency changes: %v", p.packages.Commands)
	}
	for _, cmd := range p.commands {
		if cmd[0] == "sudo" || cmd[0] == "sh" {
			t.Fatalf("build commands must run as the user without a shell: %v", cmd)
		}
	}
	if !slices.Contains(p.commands[0], "--branch=v0.41.0") || !slices.Contains(p.commands[0], mpvRepository) {
		t.Fatalf("build must select the stable upstream release: %v", p.commands[0])
	}
	if !slices.Contains(p.commands[1], "--branch=v"+placeboVersion) || !slices.Contains(p.commands[1], "--recursive") {
		t.Fatalf("libplacebo must use a compatible recursive checkout: %v", p.commands[1])
	}
	for _, flag := range []string{"-Dsixel=enabled", "-Dlua=enabled", "-Dalsa=enabled", "-Dpulse=enabled", "--wrap-mode=nodownload"} {
		if !slices.Contains(p.commands[2], flag) {
			t.Fatalf("missing required build feature %s", flag)
		}
	}
	ready, err := newSourcePlan("0.41.0", root, root, dependencyPlan{}, false, missing)
	if err != nil || len(ready.packages.Commands) != 0 {
		t.Fatalf("ready build must not need sudo: %v, %v", ready, err)
	}
	for _, bad := range []string{"", "../0.41.0", "0.41.0;touch /tmp/sentinel", "0.41.0-rc1", "0.41.0+git"} {
		if _, err := newSourcePlan(bad, root, root, dependencyPlan{}, true, missing); err == nil {
			t.Fatalf("accepted invalid stable release %q", bad)
		}
	}
}

func TestActivateBuildPreservesPlayer(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("source installation is only available on apt-based Linux")
	}
	for _, kind := range []string{"absent", "file", "symlink", "directory", "existing backup"} {
		t.Run(kind, func(t *testing.T) {
			dir := t.TempDir()
			link, backup, binary := filepath.Join(dir, "mpv"), filepath.Join(dir, "previous"), filepath.Join(dir, "built")
			if err := os.WriteFile(binary, []byte("new player"), 0o700); err != nil {
				t.Fatal(err)
			}
			switch kind {
			case "file", "existing backup":
				if err := os.WriteFile(link, []byte("old player"), 0o700); err != nil {
					t.Fatal(err)
				}
			case "symlink":
				if err := os.Symlink("original-player", link); err != nil {
					t.Fatal(err)
				}
			case "directory":
				if err := os.Mkdir(link, 0o700); err != nil {
					t.Fatal(err)
				}
			}
			if kind == "existing backup" {
				if err := os.WriteFile(backup, []byte("previous backup"), 0o600); err != nil {
					t.Fatal(err)
				}
			}
			backedUp, err := activateBuild(binary, link, backup)
			if kind == "directory" || kind == "existing backup" {
				if err == nil {
					t.Fatal("unexpected replacement of existing directory/backup")
				}
				if kind == "existing backup" {
					old, _ := os.ReadFile(link)
					if string(old) != "old player" {
						t.Fatal("failed activation changed the previous player")
					}
				}
				return
			}
			if err != nil || backedUp != (kind != "absent") {
				t.Fatalf("activation: backup=%v err=%v", backedUp, err)
			}
			if target, err := os.Readlink(link); err != nil || target != binary {
				t.Fatalf("active player=%q, %v", target, err)
			}
			switch kind {
			case "symlink":
				if target, err := os.Readlink(backup); err != nil || target != "original-player" {
					t.Fatalf("previous symlink not preserved: %q, %v", target, err)
				}
			case "file":
				if old, _ := os.ReadFile(backup); string(old) != "old player" {
					t.Fatalf("previous executable not preserved: %q", old)
				}
			}
			if entries, _ := filepath.Glob(backup + ".new"); len(entries) != 0 {
				t.Fatal("temporary activation symlink left behind")
			}
		})
	}
}

func TestDisplayCommandQuotesPaths(t *testing.T) {
	got := displayCommand([]string{"meson", "setup", "/tmp/Phil's files/$(touch sentinel)"})
	if !strings.Contains(got, "'\"'\"'") || !strings.Contains(got, "$(touch sentinel)'") {
		t.Fatalf("path must be safely quoted for review: %s", got)
	}
}
