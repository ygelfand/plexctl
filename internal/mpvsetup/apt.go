package mpvsetup

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"regexp"
	"strings"
	"time"
)

type queryFunc func(context.Context, string, ...string) ([]byte, error)

func queryCommand(ctx context.Context, name string, args ...string) ([]byte, error) {
	ctx, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()
	cmd := exec.CommandContext(ctx, name, args...)
	cmd.Env = append(os.Environ(), "LC_ALL=C")
	cmd.WaitDelay = time.Second
	return cmd.Output()
}

type aptPackage struct {
	installed string
	candidate string
}

func aptPackages(ctx context.Context, query queryFunc, names []string) (map[string]aptPackage, error) {
	out, err := query(ctx, "apt-cache", append([]string{"policy"}, names...)...)
	if err != nil {
		return nil, fmt.Errorf("check apt package versions: %w", err)
	}
	packages := make(map[string]aptPackage)
	var name string
	for line := range strings.SplitSeq(string(out), "\n") {
		if line != "" && line[0] != ' ' && strings.HasSuffix(line, ":") {
			// Multiarch packages may be printed as libfoo-dev:arm64.
			name = strings.SplitN(strings.TrimSuffix(line, ":"), ":", 2)[0]
		}
		fields := strings.Fields(line)
		if name == "" || len(fields) != 2 {
			continue
		}
		value := fields[1]
		if value == "(none)" {
			value = ""
		}
		p := packages[name]
		switch fields[0] {
		case "Installed:":
			p.installed = value
		case "Candidate:":
			p.candidate = value
		}
		packages[name] = p
	}
	return packages, nil
}

var aptVersionPattern = regexp.MustCompile(`^(?:[0-9]+:)?([0-9]+)\.([0-9]+)(?:\.([0-9]+))?`)

// Debian epochs and revisions do not describe the upstream feature version.
// In particular, FFmpeg 7:5.1 is still too old for mpv's FFmpeg >= 6.1 API.
func upstreamAPT(v string) string {
	m := aptVersionPattern.FindStringSubmatch(v)
	if m == nil {
		return ""
	}
	patch := m[3]
	if patch == "" {
		patch = "0"
	}
	result := m[1] + "." + m[2] + "." + patch
	if strings.Contains(v, "~") {
		result += "-rc"
	}
	return result
}

func olderAPT(v, minimum string) bool {
	upstream := upstreamAPT(v)
	return upstream == "" || older(upstream, minimum)
}

func availableVersion(v string) string {
	if v == "" {
		return "(unavailable)"
	}
	return v
}

type buildDependency struct {
	name    string
	minimum string
}

// These cover mpv 0.41's required libraries and plexctl's window/terminal
// playback, audio, subtitles, Lua controls, and common hardware decoding.
// Meson remains the authority on requirements for newer upstream releases.
var mpvBuildDependencies = []buildDependency{
	{"build-essential", ""}, {"git", ""}, {"ca-certificates", ""},
	{"meson", "1.3.0"}, {"ninja-build", ""}, {"pkg-config", ""},
	{"libavcodec-dev", "6.1.0"}, {"libavfilter-dev", "6.1.0"},
	{"libavformat-dev", "6.1.0"}, {"libavutil-dev", "6.1.0"},
	{"libswresample-dev", "6.1.0"}, {"libswscale-dev", "6.1.0"},
	{"libass-dev", "0.12.2"}, {"liblua5.2-dev", ""}, {"libsixel-dev", ""},
	{"libjpeg-dev", ""}, {"libuchardet-dev", ""}, {"zlib1g-dev", ""},
	{"libasound2-dev", ""}, {"libpulse-dev", ""},
	{"libx11-dev", ""}, {"libxrandr-dev", ""}, {"libxext-dev", ""},
	{"libxss-dev", ""}, {"libxpresent-dev", ""}, {"libxv-dev", ""},
	{"libgl-dev", ""}, {"libegl-dev", ""}, {"libdrm-dev", ""}, {"libgbm-dev", ""},
	{"libwayland-dev", ""}, {"wayland-protocols", ""}, {"libxkbcommon-dev", ""},
	{"libva-dev", ""}, {"libvdpau-dev", ""},
}

const placeboVersion = "6.338.2"

var placeboBuildDependencies = []buildDependency{
	{"python3-jinja2", ""}, {"libvulkan-dev", ""},
	{"libshaderc-dev", ""}, {"liblcms2-dev", ""},
}

type dependencyPlan struct {
	required []buildDependency
	missing  []string
	placebo  bool
}

func (d buildDependency) satisfied(v string) bool {
	return v != "" && (d.minimum == "" || !olderAPT(v, d.minimum))
}

func sourceDependencies(ctx context.Context, query queryFunc) (dependencyPlan, error) {
	deps := append([]buildDependency(nil), mpvBuildDependencies...)
	placebo := buildDependency{"libplacebo-dev", placeboVersion}
	names := make([]string, 0, len(deps)+len(placeboBuildDependencies)+1)
	for _, dep := range deps {
		names = append(names, dep.name)
	}
	names = append(names, placebo.name)
	for _, dep := range placeboBuildDependencies {
		names = append(names, dep.name)
	}
	packages, err := aptPackages(ctx, query, names)
	if err != nil {
		return dependencyPlan{}, err
	}
	p := packages[placebo.name]
	fallback := !placebo.satisfied(p.installed) && !placebo.satisfied(p.candidate)
	if fallback {
		deps = append(deps, placeboBuildDependencies...)
	} else {
		deps = append(deps, placebo)
	}
	plan := dependencyPlan{required: deps, placebo: fallback}
	var unavailable []string
	for _, dep := range deps {
		p := packages[dep.name]
		if dep.satisfied(p.installed) {
			continue
		}
		plan.missing = append(plan.missing, dep.name)
		if !dep.satisfied(p.candidate) {
			requirement := dep.name
			if dep.minimum != "" {
				requirement += " >= " + dep.minimum
			}
			unavailable = append(unavailable, requirement+" (apt: "+availableVersion(p.candidate)+")")
		}
	}
	if len(unavailable) > 0 {
		return plan, fmt.Errorf("build dependencies unavailable: %s; refresh apt indexes or use a distribution/repository with these development packages, then retry; build instructions: https://github.com/mpv-player/mpv#compilation", strings.Join(unavailable, ", "))
	}
	return plan, nil
}
