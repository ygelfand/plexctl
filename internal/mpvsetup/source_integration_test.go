//go:build integration && linux

package mpvsetup

import (
	"bytes"
	"context"
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"syscall"
	"testing"
	"time"
)

func fakeSourceTools(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	t.Setenv("PATH", dir)
	t.Setenv("MPV_TEST_DIR", dir)
	t.Setenv("MPV_TEST_FAILURE", "")
	write := func(name, body string) {
		t.Helper()
		if err := os.WriteFile(filepath.Join(dir, name), []byte(body), 0o700); err != nil {
			t.Fatal(err)
		}
	}
	packages := availableDependencies()
	packages["mpv"] = aptPackage{"0.37.0-1ubuntu4", "0.37.0-1ubuntu4"}
	write("policy-after", dependencyPolicy(packages))
	packages["meson"] = aptPackage{"", "1.3.2-1ubuntu1"}
	packages["libsixel-dev"] = aptPackage{"", "1.10.3-1"}
	write("policy", dependencyPolicy(packages))
	write("apt-cache", "#!/bin/sh\nexec /bin/cat \"$MPV_TEST_DIR/policy\"\n")
	write("sudo", "#!/bin/sh\nexec \"$@\"\n")
	write("apt-get", `#!/bin/sh
printf 'apt-get %s\n' "$*" >> "$MPV_TEST_DIR/commands"
if [ "$MPV_TEST_FAILURE" = apt ]; then exit 7; fi
if [ "$1" = install ]; then
  /bin/cp "$MPV_TEST_DIR/policy-after" "$MPV_TEST_DIR/policy"
  for argument; do
    if [ "$argument" = mpv ]; then /bin/cp "$MPV_TEST_DIR/built-player" "$MPV_TEST_DIR/mpv"; fi
  done
fi
`)
	write("git", `#!/bin/sh
printf 'git %s\n' "$*" >> "$MPV_TEST_DIR/commands"
if [ "$MPV_TEST_FAILURE" = clone ]; then exit 8; fi
for destination; do :; done
/bin/mkdir -p "$destination/subprojects"
`)
	write("meson", `#!/bin/sh
if [ "$1" = --version ]; then printf '1.3.2\n'; exit 0; fi
printf 'meson %s\n' "$*" >> "$MPV_TEST_DIR/commands"
case "$1" in
  setup)
    for argument; do
      case "$argument" in --prefix=*) printf '%s' "${argument#--prefix=}" > "$MPV_TEST_DIR/prefix";; esac
    done
    ;;
  compile)
    printf '%s' "$$" > "$MPV_TEST_DIR/build-pid"
    if [ "$MPV_TEST_FAILURE" = compile ]; then printf 'compiler failed\n' >&2; exit 9; fi
    if [ "$MPV_TEST_FAILURE" = cancel ]; then
      /bin/sleep 30 &
      printf '%s' "$!" > "$MPV_TEST_DIR/child-pid"
      wait
    fi
    ;;
  install)
    if [ "$MPV_TEST_FAILURE" = install ]; then exit 10; fi
    prefix=$(/bin/cat "$MPV_TEST_DIR/prefix")
    /bin/mkdir -p "$prefix/bin"
    /bin/cp "$MPV_TEST_DIR/built-player" "$prefix/bin/mpv"
    /bin/chmod 700 "$prefix/bin/mpv"
    ;;
esac
`)
	write("built-player", `#!/bin/sh
case "$*" in
  '--no-config --version')
    if [ "$MPV_TEST_FAILURE" = version ]; then printf 'mpv 0.37.0\n'; else printf 'mpv 0.41.0\n'; fi
    ;;
  '--no-config --vo=help')
    printf 'Available video outputs:\n  kitty Kitty\n  tct Text\n  gpu OpenGL\n'
    if [ "$MPV_TEST_FAILURE" != sixel ]; then printf '  sixel Sixel\n'; fi
    ;;
esac
`)
	write("mpv", "#!/bin/sh\nprintf 'mpv 0.37.0\\n'\n")
	cache, err := json.Marshal(releaseCache{Version: "0.41.0", Checked: time.Now()})
	if err != nil {
		t.Fatal(err)
	}
	write("mpv-release.json", string(cache))
	return dir
}

func TestEnsureAptSourceApproval(t *testing.T) {
	for _, piped := range []bool{false, true} {
		t.Run(map[bool]string{false: "declined", true: "piped yes"}[piped], func(t *testing.T) {
			dir := fakeSourceTools(t)
			var out bytes.Buffer
			opts := Options{CacheDir: dir, Output: &out, Error: &out, Required: true, Install: true}
			if piped {
				opts.Input = strings.NewReader("yes\n")
			} else {
				opts.Input = approvalTerminal(t, "n\n")
			}
			if err := Ensure(t.Context(), opts); err != nil {
				t.Fatal(err)
			}
			if _, err := os.Stat(filepath.Join(dir, "commands")); !os.IsNotExist(err) {
				t.Fatal("declined/piped input executed an installation command")
			}
			if !piped {
				for _, want := range []string{"Apt offers mpv 0.37.0", "source", "meson >= 1.3.0", "libsixel-dev", "Proceed with these commands? [y/N]"} {
					if !strings.Contains(out.String(), want) {
						t.Fatalf("missing %q in preflight:\n%s", want, out.String())
					}
				}
			}
		})
	}
}

func TestEnsureAptCandidateAndPostInstallFallback(t *testing.T) {
	for _, stale := range []bool{false, true} {
		t.Run(map[bool]string{false: "current candidate", true: "stale candidate"}[stale], func(t *testing.T) {
			dir := fakeSourceTools(t)
			packages := availableDependencies()
			packages["mpv"] = aptPackage{"0.37.0-1ubuntu4", "0.41.0-1"}
			if err := os.WriteFile(filepath.Join(dir, "policy"), []byte(dependencyPolicy(packages)), 0o600); err != nil {
				t.Fatal(err)
			}
			answer := "yes\n"
			if stale {
				// The repository or PATH did not supply the advertised version.
				// Approving apt does not implicitly approve a subsequent build.
				t.Setenv("MPV_TEST_FAILURE", "version")
				answer += "n\n"
			}
			var out bytes.Buffer
			err := Ensure(t.Context(), Options{CacheDir: dir, Input: approvalTerminal(t, answer), Output: &out, Error: &out, Required: true, Install: true})
			if err != nil {
				t.Fatalf("Ensure: %v\n%s", err, out.String())
			}
			commands, _ := os.ReadFile(filepath.Join(dir, "commands"))
			if !strings.Contains(string(commands), "apt-get install --yes mpv") || strings.Contains(string(commands), "git clone") {
				t.Fatalf("unexpected installations: %s", commands)
			}
			prompts := strings.Count(out.String(), "Proceed with these commands?")
			if stale && (prompts != 2 || !strings.Contains(out.String(), "source build declined")) {
				t.Fatalf("stale repository must offer a separately approved build:\n%s", out.String())
			}
			if !stale && (prompts != 1 || strings.Contains(out.String(), "Build stable mpv")) {
				t.Fatalf("current apt candidate should use only the package manager:\n%s", out.String())
			}
		})
	}
}

func TestRunSourceBuild(t *testing.T) {
	for _, failure := range []string{"", "apt", "clone", "compile", "install", "version", "sixel"} {
		t.Run("failure="+failure, func(t *testing.T) {
			dir := fakeSourceTools(t)
			t.Setenv("MPV_TEST_FAILURE", failure)
			deps, err := sourceDependencies(t.Context(), queryCommand)
			if err != nil {
				t.Fatal(err)
			}
			p, err := newSourcePlan("0.41.0", filepath.Join(dir, "user's files $(literal)"), dir, deps, false, exec.LookPath)
			if err != nil {
				t.Fatal(err)
			}
			if err := os.MkdirAll(filepath.Dir(p.link), 0o755); err != nil {
				t.Fatal(err)
			}
			if err := os.WriteFile(p.link, []byte("previous player"), 0o700); err != nil {
				t.Fatal(err)
			}
			var out bytes.Buffer
			opts := Options{Input: strings.NewReader(""), Output: &out, Error: &out}
			err = runSourceBuild(t.Context(), p, opts)
			if failure != "" {
				if err == nil {
					t.Fatalf("%s failure was accepted", failure)
				}
				old, _ := os.ReadFile(p.link)
				if string(old) != "previous player" {
					t.Fatal("failed build replaced previous player")
				}
				if _, err := os.Stat(p.prefix); !os.IsNotExist(err) {
					t.Fatal("incomplete install prefix was not removed")
				}
				if failure == "compile" {
					log, _ := os.ReadFile(filepath.Join(p.work, "build.log"))
					if !strings.Contains(string(log), "compiler failed") {
						t.Fatal("failure log was not preserved")
					}
				}
				return
			}
			if err != nil {
				t.Fatalf("build: %v\n%s", err, out.String())
			}
			if old, _ := os.ReadFile(p.backup); string(old) != "previous player" {
				t.Fatal("successful installation lost previous player")
			}
			s, err := installed(t.Context())
			if err != nil || s.Installed != "0.41.0" || s.Path != p.link {
				t.Fatalf("new player must be active for immediate playback: %+v, %v", s, err)
			}
			commands, _ := os.ReadFile(filepath.Join(dir, "commands"))
			for _, want := range []string{"apt-get install --yes --no-remove meson libsixel-dev", "--branch=v0.41.0", "meson setup", "meson compile", "meson install"} {
				if !strings.Contains(string(commands), want) {
					t.Fatalf("missing build step %q in %s", want, commands)
				}
			}
		})
	}
}

func TestSourceBuildCancellationStopsCompilers(t *testing.T) {
	dir := fakeSourceTools(t)
	t.Setenv("MPV_TEST_FAILURE", "cancel")
	deps, err := sourceDependencies(t.Context(), queryCommand)
	if err != nil {
		t.Fatal(err)
	}
	p, err := newSourcePlan("0.41.0", dir, dir, deps, true, exec.LookPath)
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithCancel(t.Context())
	defer cancel()
	// Use files for output: stdout/stderr may be copied concurrently by os/exec.
	log, err := os.CreateTemp(dir, "output")
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = log.Close() }()
	done := make(chan error, 1)
	go func() { done <- runSourceBuild(ctx, p, Options{Input: strings.NewReader(""), Output: log, Error: log}) }()
	var pid []byte
	deadline := time.Now().Add(3 * time.Second)
	for time.Now().Before(deadline) {
		pid, _ = os.ReadFile(filepath.Join(dir, "child-pid"))
		if len(pid) > 0 {
			break
		}
		time.Sleep(10 * time.Millisecond)
	}
	cancel()
	select {
	case err := <-done:
		if err == nil {
			t.Fatal("canceled build succeeded")
		}
	case <-time.After(3 * time.Second):
		t.Fatal("build ignored cancellation")
	}
	if len(pid) == 0 {
		t.Fatal("fake compiler never started")
	}
	status, err := os.ReadFile(filepath.Join("/proc", string(pid), "status"))
	if err == nil && !strings.Contains(string(status), "State:\tZ") {
		t.Fatalf("compiler survived cancellation:\n%s", status)
	}
	if _, err := os.Lstat(p.link); !os.IsNotExist(err) {
		t.Fatal("canceled build activated a player")
	}
}

func TestSourceBuildSIGTERMCleansUp(t *testing.T) {
	dir := fakeSourceTools(t)
	t.Setenv("MPV_TEST_FAILURE", "cancel")
	t.Setenv("PLEXCTL_BUILD_SIGNAL_HELPER", "1")
	binary, err := os.Executable()
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithTimeout(t.Context(), 10*time.Second)
	defer cancel()
	cmd := exec.CommandContext(ctx, binary, "-test.run=^TestSourceBuildSignalHelper$")
	var output bytes.Buffer
	cmd.Stdout, cmd.Stderr = &output, &output
	if err := cmd.Start(); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		_ = cmd.Process.Kill()
		if data, err := os.ReadFile(filepath.Join(dir, "build-pid")); err == nil {
			if pid, err := strconv.Atoi(string(data)); err == nil && pid > 0 {
				_ = syscall.Kill(-pid, syscall.SIGKILL)
			}
		}
		if cmd.ProcessState == nil {
			_ = cmd.Wait()
		}
	})
	deadline := time.Now().Add(3 * time.Second)
	for time.Now().Before(deadline) {
		if data, _ := os.ReadFile(filepath.Join(dir, "child-pid")); len(data) > 0 {
			break
		}
		time.Sleep(10 * time.Millisecond)
	}
	if _, err := os.Stat(filepath.Join(dir, "child-pid")); err != nil {
		t.Fatal("compiler did not start")
	}
	if err := cmd.Process.Signal(syscall.SIGTERM); err != nil {
		t.Fatal(err)
	}
	if err := cmd.Wait(); err != nil {
		t.Fatalf("SIGTERM bypassed build cleanup: %v\n%s", err, output.String())
	}
	if prefix, err := os.ReadFile(filepath.Join(dir, "prefix")); err != nil {
		t.Fatal(err)
	} else if _, err := os.Stat(string(prefix)); !os.IsNotExist(err) {
		t.Fatal("interrupted build left its incomplete installation behind")
	}
}

func TestSourceBuildSignalHelper(t *testing.T) {
	if os.Getenv("PLEXCTL_BUILD_SIGNAL_HELPER") != "1" {
		return
	}
	dir := os.Getenv("MPV_TEST_DIR")
	deps, err := sourceDependencies(t.Context(), queryCommand)
	if err != nil {
		t.Fatal(err)
	}
	p, err := newSourcePlan("0.41.0", dir, dir, deps, true, exec.LookPath)
	if err != nil {
		t.Fatal(err)
	}
	err = runSourceBuild(t.Context(), p, Options{Input: strings.NewReader(""), Output: os.Stdout, Error: os.Stderr})
	if err == nil {
		t.Fatal("terminated build succeeded")
	}
}
