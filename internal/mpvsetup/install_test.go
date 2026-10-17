package mpvsetup

import (
	"bytes"
	"os/exec"
	"strings"
	"testing"
)

func TestInstallRequiresExplicitApproval(t *testing.T) {
	for _, tt := range []struct {
		input string
		want  bool
	}{
		{"\n", false}, {"n\n", false}, {"no\n", false}, {"", false}, {"yes", false},
		{"maybe\n", false}, {"y\n", true}, {"YES\n", true}, {" yes \r\n", true},
	} {
		t.Run(tt.input, func(t *testing.T) {
			var out bytes.Buffer
			ok, err := confirm(strings.NewReader(tt.input), &out)
			if err != nil || ok != tt.want {
				t.Fatalf("approval: %v, %v", ok, err)
			}
			if !strings.Contains(out.String(), "[y/N]") {
				t.Fatal("prompt did not default to declining")
			}
		})
	}
	if interactive(strings.NewReader("yes\n")) {
		t.Fatal("piped yes must not authorize installation")
	}
}

func TestInstallPlanScope(t *testing.T) {
	for _, tt := range []struct {
		goos, manager string
		root          bool
		want          string
	}{
		{"linux", "apt-get", false, "sudo apt-get update\nsudo apt-get install --yes mpv"},
		{"linux", "dnf", true, "dnf install --refresh --assumeyes mpv"},
		{"linux", "pacman", false, "sudo pacman -S --needed mpv"},
		{"darwin", "brew", false, "brew update\nbrew install mpv"},
	} {
		t.Run(tt.manager, func(t *testing.T) {
			lookup := func(name string) (string, error) {
				if name == tt.manager || name == "sudo" {
					return "/bin/" + name, nil
				}
				return "", exec.ErrNotFound
			}
			plan, err := installPlan(tt.goos, lookup, true, tt.root)
			if err != nil {
				t.Fatal(err)
			}
			var commands []string
			for _, args := range plan.Commands {
				commands = append(commands, strings.Join(args, " "))
			}
			if got := strings.Join(commands, "\n"); got != tt.want {
				t.Fatalf("plan:\n%s", got)
			}
		})
	}
	missing := func(string) (string, error) { return "", exec.ErrNotFound }
	if _, err := installPlan("windows", missing, true, false); err == nil {
		t.Fatal("unsupported platform must provide manual instructions")
	}
	if _, err := installPlan("linux", func(name string) (string, error) {
		if name == "apt-get" {
			return "/usr/bin/apt-get", nil
		}
		return missing(name)
	}, true, false); err == nil {
		t.Fatal("missing sudo must not attempt privileged installation")
	}
}
