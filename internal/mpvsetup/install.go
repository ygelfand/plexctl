package mpvsetup

import (
	"bufio"
	"context"
	"fmt"
	"io"
	"os"
	"os/exec"
	"runtime"
	"strings"

	"golang.org/x/term"
)

const installHelp = "https://mpv.io/installation/"

// Plan contains only fixed package-manager arguments, never release metadata.
type Plan struct {
	Commands [][]string
	apt      bool
}

func installPlan(goos string, lookup func(string) (string, error), missing, root bool) (Plan, error) {
	has := func(name string) bool { _, err := lookup(name); return err == nil }
	var commands [][]string
	apt := false
	privileged := goos != "windows"
	switch {
	case (goos == "darwin" || goos == "linux") && has("brew"):
		privileged = false
		action := "upgrade"
		if missing {
			action = "install"
		}
		commands = [][]string{{"brew", "update"}, {"brew", action, "mpv"}}
	case goos == "linux" && has("apt-get"):
		apt = true
		commands = [][]string{{"apt-get", "update"}, {"apt-get", "install", "--yes", "mpv"}}
	case goos == "linux" && has("dnf"):
		commands = [][]string{{"dnf", "install", "--refresh", "--assumeyes", "mpv"}}
	case goos == "linux" && has("pacman"):
		// Do not refresh Arch's databases then perform an unsupported partial upgrade.
		commands = [][]string{{"pacman", "-S", "--needed", "mpv"}}
	case goos == "linux" && has("zypper"):
		commands = [][]string{{"zypper", "refresh"}, {"zypper", "--non-interactive", "install", "mpv"}}
	case goos == "linux" && has("apk"):
		commands = [][]string{{"apk", "add", "--upgrade", "--update-cache", "mpv"}}
	default:
		return Plan{}, fmt.Errorf("no supported mpv package manager found; install mpv from %s", installHelp)
	}
	if privileged && !root {
		if !has("sudo") {
			return Plan{}, fmt.Errorf("installing mpv requires administrator access; see %s", installHelp)
		}
		for i := range commands {
			commands[i] = append([]string{"sudo"}, commands[i]...)
		}
	}
	return Plan{Commands: commands, apt: apt}, nil
}

type Options struct {
	Input    io.Reader
	Output   io.Writer
	Error    io.Writer
	CacheDir string
	// Required is false for browsing the TUI without a working player.
	Required bool
	// Refresh bypasses the daily release cache for the explicit mpv command.
	Refresh bool
	// Install offers installation; otherwise only report the current state.
	Install bool
	// Quiet suppresses the report when mpv is current.
	Quiet bool
}

func interactive(r io.Reader) bool {
	f, ok := r.(interface{ Fd() uintptr })
	return ok && term.IsTerminal(int(f.Fd()))
}

func confirm(in io.Reader, out io.Writer) (bool, error) {
	if _, err := fmt.Fprint(out, "Proceed with these commands? [y/N] "); err != nil {
		return false, err
	}
	line, err := bufio.NewReader(in).ReadString('\n')
	if err != nil {
		// EOF, including an unterminated 'yes', is never approval.
		return false, nil
	}
	answer := strings.ToLower(strings.TrimSpace(line))
	return answer == "y" || answer == "yes", nil
}

func runPlan(ctx context.Context, p Plan, opts Options) error {
	for _, args := range p.Commands {
		cmd := exec.CommandContext(ctx, args[0], args[1:]...)
		cmd.Stdin, cmd.Stdout, cmd.Stderr = opts.Input, opts.Output, opts.Error
		if err := cmd.Run(); err != nil {
			return fmt.Errorf("mpv installation command failed: %w", err)
		}
	}
	return nil
}

// Ensure checks before the TUI takes over stdin. No command can run without an
// explicit affirmative answer from an interactive terminal.
func Ensure(ctx context.Context, opts Options) error {
	s, err := Check(ctx, opts.CacheDir, opts.Refresh)
	if err != nil {
		return dependencyError(opts, err)
	}
	if !opts.Quiet || s.NeedsInstall() || s.CheckErr != nil {
		if s.Path == "" {
			_, _ = fmt.Fprintln(opts.Output, "mpv is not installed or is not on PATH.")
		} else {
			_, _ = fmt.Fprintf(opts.Output, "mpv %s (%s)\n", s.Installed, s.Path)
		}
		if s.Latest != "" {
			_, _ = fmt.Fprintf(opts.Output, "Latest stable release: %s\n", s.Latest)
		}
		if s.CheckErr != nil {
			_, _ = fmt.Fprintf(opts.Error, "Could not verify the latest mpv release: %v.\n", s.CheckErr)
		}
	}
	if !s.NeedsInstall() {
		return nil
	}
	missing := s.Path == ""
	if !opts.Install || !interactive(opts.Input) {
		if missing {
			return dependencyError(opts, fmt.Errorf("mpv is required for playback; run 'plexctl mpv install' in a terminal or see %s", installHelp))
		}
		_, _ = fmt.Fprintln(opts.Error, "mpv update available; run 'plexctl mpv install' to approve an update.")
		return nil
	}
	// Inspect apt before offering a package known to be too old. This also lets
	// users with all build dependencies installed build without needing sudo.
	_, aptErr := exec.LookPath("apt-get")
	_, brewErr := exec.LookPath("brew")
	if runtime.GOOS == "linux" && aptErr == nil && brewErr != nil && s.Latest != "" {
		packages, queryErr := aptPackages(ctx, queryCommand, []string{"mpv"})
		candidate := packages["mpv"].candidate
		if queryErr == nil && (candidate == "" || olderAPT(candidate, s.Latest)) {
			_, _ = fmt.Fprintf(opts.Output, "Apt offers mpv %s; building stable mpv %s from source instead.\n", availableVersion(candidate), s.Latest)
			return dependencyError(opts, offerSourceBuild(ctx, s, opts))
		}
	}
	plan, err := installPlan(runtime.GOOS, exec.LookPath, missing, os.Geteuid() == 0)
	if err != nil {
		return dependencyError(opts, err)
	}
	var description strings.Builder
	description.WriteString("Install the newest mpv available from your configured package repositories:\n")
	for _, args := range plan.Commands {
		fmt.Fprintf(&description, "  %s\n", strings.Join(args, " "))
	}
	description.WriteString("Repositories may lag the upstream release; plexctl will verify the version afterward.\n")
	if _, err := io.WriteString(opts.Output, description.String()); err != nil {
		return fmt.Errorf("show mpv installation commands: %w", err)
	}
	approved, err := confirm(opts.Input, opts.Output)
	if err != nil {
		return err
	}
	if !approved {
		if missing {
			return dependencyError(opts, fmt.Errorf("mpv installation declined; playback requires mpv"))
		}
		_, _ = fmt.Fprintln(opts.Output, "mpv update declined; using the installed version.")
		return nil
	}
	if err := runPlan(ctx, plan, opts); err != nil {
		return dependencyError(opts, err)
	}
	after, err := installed(ctx)
	if err != nil {
		return dependencyError(opts, err)
	}
	if after.Path == "" {
		return dependencyError(opts, fmt.Errorf("mpv is still not on PATH after installation; restart your shell or see %s", installHelp))
	}
	_, _ = fmt.Fprintf(opts.Output, "Using mpv %s (%s).\n", after.Installed, after.Path)
	if s.Latest != "" && older(after.Installed, s.Latest) {
		if plan.apt {
			_, _ = fmt.Fprintln(opts.Output, "The apt installation is still outdated; a source build is available.")
			after.Latest = s.Latest
			return dependencyError(opts, offerSourceBuild(ctx, after, opts))
		}
		return dependencyError(opts, fmt.Errorf("mpv %s is still older than %s; your repositories may be behind or another mpv may take precedence on PATH; see %s", after.Installed, s.Latest, installHelp))
	}
	return nil
}

func dependencyError(opts Options, err error) error {
	if err == nil {
		return nil
	}
	if opts.Required {
		return err
	}
	_, _ = fmt.Fprintf(opts.Error, "%v. You can continue browsing.\n", err)
	return nil
}
