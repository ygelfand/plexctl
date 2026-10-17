package mpvsetup

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"sync"
	"time"
)

const mpvRepository = "https://github.com/mpv-player/mpv.git"
const placeboRepository = "https://code.videolan.org/videolan/libplacebo.git"

type sourcePlan struct {
	version, work, prefix, link, backup string
	deps                                dependencyPlan
	packages                            Plan
	commands                            [][]string
}

func newSourcePlan(v, home, cacheDir string, deps dependencyPlan, root bool, lookup func(string) (string, error)) (sourcePlan, error) {
	if !stableVersion(v) {
		return sourcePlan{}, fmt.Errorf("a verified stable mpv release is required for a source build")
	}
	v = strings.TrimPrefix(v, "v")
	if !filepath.IsAbs(home) || !filepath.IsAbs(cacheDir) {
		return sourcePlan{}, fmt.Errorf("source build requires absolute home and cache directories")
	}
	id := v + "-" + strconv.FormatInt(time.Now().UnixNano(), 36)
	p := sourcePlan{
		version: v, deps: deps,
		work:   filepath.Join(cacheDir, "mpv-build-"+id),
		prefix: filepath.Join(home, ".local", "opt", "plexctl-mpv-"+id),
		link:   filepath.Join(home, ".local", "bin", "mpv"),
	}
	p.backup = p.link + ".before-plexctl-" + id
	if len(deps.missing) > 0 {
		p.packages.Commands = [][]string{
			{"apt-get", "update"},
			append([]string{"apt-get", "install", "--yes", "--no-remove"}, deps.missing...),
		}
		if !root {
			if _, err := lookup("sudo"); err != nil {
				return sourcePlan{}, fmt.Errorf("installing missing build dependencies requires sudo or administrator access")
			}
			for i := range p.packages.Commands {
				p.packages.Commands[i] = append([]string{"sudo"}, p.packages.Commands[i]...)
			}
		}
	}
	source := filepath.Join(p.work, "mpv")
	build := filepath.Join(source, "build")
	p.commands = [][]string{{"git", "clone", "--depth=1", "--branch=v" + v, mpvRepository, source}}
	if deps.placebo {
		p.commands = append(p.commands, []string{
			"git", "clone", "--depth=1", "--recursive", "--branch=v" + placeboVersion,
			placeboRepository, filepath.Join(source, "subprojects", "libplacebo"),
		})
	}
	setup := []string{"meson", "setup", build, source, "--prefix=" + p.prefix,
		"--buildtype=release", "--wrap-mode=nodownload", "-Dlibmpv=false",
		"-Dlua=enabled", "-Dsixel=enabled", "-Dalsa=enabled", "-Dpulse=enabled",
		"-Dx11=enabled", "-Dgl=enabled", "-Degl=enabled", "-Djpeg=enabled",
		"-Duchardet=enabled", "-Dzlib=enabled", "-Dmanpage-build=disabled"}
	if deps.placebo {
		setup = append(setup, "--force-fallback-for=libplacebo", "-Dlibplacebo:default_library=static")
	}
	p.commands = append(p.commands, setup,
		[]string{"meson", "compile", "-C", build, "-j", strconv.Itoa(min(4, runtime.NumCPU()))},
		[]string{"meson", "install", "-C", build, "--no-rebuild"})
	return p, nil
}

func describeDependencies(out io.Writer, deps dependencyPlan) {
	_, _ = fmt.Fprintln(out, "Build dependencies (Meson is installed through apt):")
	for _, dep := range deps.required {
		requirement := dep.name
		if dep.minimum != "" {
			requirement += " >= " + dep.minimum
		}
		_, _ = fmt.Fprintln(out, "  "+requirement)
	}
	if len(deps.missing) == 0 {
		_, _ = fmt.Fprintln(out, "All required apt packages are already installed.")
	} else {
		_, _ = fmt.Fprintf(out, "Packages to install or upgrade: %s\n", strings.Join(deps.missing, " "))
	}
	if deps.placebo {
		_, _ = fmt.Fprintf(out, "Apt cannot provide a suitable libplacebo; build v%s as a static Meson subproject.\n", placeboVersion)
	}
}

func offerSourceBuild(ctx context.Context, s Status, opts Options) error {
	deps, err := sourceDependencies(ctx, queryCommand)
	if len(deps.required) > 0 {
		describeDependencies(opts.Output, deps)
	}
	if err != nil {
		return err
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return err
	}
	cacheDir := opts.CacheDir
	if cacheDir == "" {
		cacheDir, err = os.UserCacheDir()
		if err != nil {
			return err
		}
		cacheDir = filepath.Join(cacheDir, "plexctl")
	}
	cacheDir, err = filepath.Abs(cacheDir)
	if err != nil {
		return err
	}
	p, err := newSourcePlan(s.Latest, home, cacheDir, deps, os.Geteuid() == 0, exec.LookPath)
	if err != nil {
		return err
	}
	var description strings.Builder
	fmt.Fprintf(&description, "Build stable mpv %s with Kitty, Sixel, text video, Lua, subtitles, and audio support:\n", p.version)
	for _, commands := range [][][]string{p.packages.Commands, p.commands} {
		for _, args := range commands {
			fmt.Fprintf(&description, "  %s\n", displayCommand(args))
		}
	}
	fmt.Fprintf(&description, "After verification, activate %s via %s.\n", filepath.Join(p.prefix, "bin", "mpv"), p.link)
	fmt.Fprintf(&description, "If %s exists, preserve it at %s.\n", p.link, p.backup)
	fmt.Fprintf(&description, "Build sources and logs: %s\nDownloading and compiling may take several minutes.\n", p.work)
	if _, err := io.WriteString(opts.Output, description.String()); err != nil {
		return fmt.Errorf("show mpv source build: %w", err)
	}
	approved, err := confirm(opts.Input, opts.Output)
	if err != nil {
		return err
	}
	if !approved {
		if s.Path == "" {
			return fmt.Errorf("mpv source build declined; playback requires mpv")
		}
		_, _ = fmt.Fprintln(opts.Output, "mpv source build declined; using the installed version.")
		return nil
	}
	return runSourceBuild(ctx, p, opts)
}

// displayCommand quotes arguments for review only. Execution always uses argv,
// never a shell, including when a home directory contains shell metacharacters.
func displayCommand(args []string) string {
	quoted := make([]string, len(args))
	for i, arg := range args {
		quoted[i] = "'" + strings.ReplaceAll(arg, "'", "'\"'\"'") + "'"
	}
	return strings.Join(quoted, " ")
}

func runSourceBuild(ctx context.Context, p sourcePlan, opts Options) error {
	ctx, stop := buildContext(ctx)
	defer stop()
	if err := runPlan(ctx, p.packages, opts); err != nil {
		return err
	}
	// Recheck after apt runs, before a potentially lengthy download and build.
	names := make([]string, 0, len(p.deps.required))
	for _, dep := range p.deps.required {
		names = append(names, dep.name)
	}
	packages, err := aptPackages(ctx, queryCommand, names)
	if err != nil {
		return err
	}
	for _, dep := range p.deps.required {
		if !dep.satisfied(packages[dep.name].installed) {
			return fmt.Errorf("build dependency %s is still missing or too old after apt installation", dep.name)
		}
	}
	meson, err := queryCommand(ctx, "meson", "--version")
	if err != nil || olderAPT(strings.TrimSpace(string(meson)), "1.3.0") {
		return fmt.Errorf("meson >= 1.3.0 must be on PATH; check that another meson is not shadowing the apt installation")
	}
	if err := os.MkdirAll(filepath.Dir(p.work), 0o755); err != nil {
		return err
	}
	if err := os.Mkdir(p.work, 0o700); err != nil {
		return err
	}
	logPath := filepath.Join(p.work, "build.log")
	log, err := os.OpenFile(logPath, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0o600)
	if err != nil {
		return err
	}
	defer func() { _ = log.Close() }()
	// This unique prefix cannot overwrite an existing installation. Leave the
	// sources/logs for diagnosis if compilation, verification, or activation fails.
	if err := os.MkdirAll(filepath.Dir(p.prefix), 0o755); err != nil {
		return err
	}
	if err := os.Mkdir(p.prefix, 0o755); err != nil {
		return err
	}
	activated := false
	defer func() {
		if !activated {
			_ = os.RemoveAll(p.prefix)
		}
	}()
	for _, args := range p.commands {
		_, _ = fmt.Fprintln(log, displayCommand(args))
		cmd := exec.CommandContext(ctx, args[0], args[1:]...)
		cmd.Dir = p.work
		cmd.Stdin = opts.Input
		var outputMu sync.Mutex
		cmd.Stdout = &buildOutput{mu: &outputMu, writer: io.MultiWriter(opts.Output, log)}
		cmd.Stderr = &buildOutput{mu: &outputMu, writer: io.MultiWriter(opts.Error, log)}
		prepareBuildCommand(cmd)
		if err := cmd.Run(); err != nil {
			return fmt.Errorf("mpv source build failed at %s: %w; see %s and mpv/build/meson-logs in %s", args[0], err, logPath, p.work)
		}
	}
	binary := filepath.Join(p.prefix, "bin", "mpv")
	if err := verifyBuild(ctx, binary, p.version); err != nil {
		return fmt.Errorf("%w; build log: %s", err, logPath)
	}
	backedUp, err := activateBuild(binary, p.link, p.backup)
	if err != nil {
		return fmt.Errorf("activate built mpv: %w", err)
	}
	activated = true
	if backedUp {
		_, _ = fmt.Fprintf(opts.Output, "Previous mpv preserved at %s.\n", p.backup)
	}
	// Make the approved player available to playback in this process even when
	// ~/.local/bin was absent from PATH. Never modify shell configuration files.
	binDir := filepath.Dir(p.link)
	resolved, _ := exec.LookPath("mpv")
	if resolved != p.link {
		if err := os.Setenv("PATH", binDir+string(os.PathListSeparator)+os.Getenv("PATH")); err != nil {
			return err
		}
		_, _ = fmt.Fprintf(opts.Output, "For future shells, put this directory first on PATH:\n  export PATH=%s:\"$PATH\"\n", displayCommand([]string{binDir}))
	}
	_, _ = fmt.Fprintf(opts.Output, "Using source-built mpv %s (%s). Build log: %s\n", p.version, p.link, logPath)
	return nil
}

// os/exec copies stdout and stderr concurrently. Serialize both destinations
// because callers may use the same non-thread-safe writer for their output.
type buildOutput struct {
	mu     *sync.Mutex
	writer io.Writer
}

func (w *buildOutput) Write(data []byte) (int, error) {
	w.mu.Lock()
	defer w.mu.Unlock()
	return w.writer.Write(data)
}

func verifyBuild(ctx context.Context, binary, minimum string) error {
	s, err := inspect(ctx, binary)
	if err != nil {
		return err
	}
	if older(s.Installed, minimum) {
		return fmt.Errorf("built mpv %s is older than requested %s", s.Installed, minimum)
	}
	out, err := queryCommand(ctx, binary, "--no-config", "--vo=help")
	if err != nil {
		return fmt.Errorf("check built mpv video outputs: %w", err)
	}
	outputs := make(map[string]bool)
	for line := range strings.SplitSeq(string(out), "\n") {
		if fields := strings.Fields(line); len(fields) > 0 {
			outputs[fields[0]] = true
		}
	}
	for _, name := range []string{"kitty", "sixel", "tct", "gpu"} {
		if !outputs[name] {
			return fmt.Errorf("built mpv lacks required %s video output", name)
		}
	}
	return nil
}

func activateBuild(binary, link, backup string) (bool, error) {
	if err := os.MkdirAll(filepath.Dir(link), 0o755); err != nil {
		return false, err
	}
	temporary := backup + ".new"
	if err := os.Symlink(binary, temporary); err != nil {
		return false, err
	}
	defer func() { _ = os.Remove(temporary) }()
	backedUp := false
	if info, err := os.Lstat(link); err == nil {
		if !info.Mode().IsRegular() && info.Mode()&os.ModeSymlink == 0 {
			return false, fmt.Errorf("%s is not a file or symlink", link)
		}
		// A hard link preserves either the previous regular file or symlink,
		// while rename below atomically switches the active path on Linux.
		if err := os.Link(link, backup); err != nil {
			return false, fmt.Errorf("preserve previous mpv: %w", err)
		}
		backedUp = true
	} else if !errors.Is(err, os.ErrNotExist) {
		return false, err
	}
	if err := os.Rename(temporary, link); err != nil {
		return backedUp, err
	}
	return backedUp, nil
}
