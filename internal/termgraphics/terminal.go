// Package termgraphics detects terminal graphics and handles tmux passthrough.
package termgraphics

import (
	"context"
	"encoding/base64"
	"fmt"
	"io"
	"log/slog"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"runtime"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/charmbracelet/x/ansi"
	"github.com/muesli/cancelreader"
	"golang.org/x/term"
)

type Capabilities struct {
	Responded bool
	Kitty     bool
	// KittySharedMemory is confirmed by a query using a local memory object;
	// environment variables cannot establish that the terminal is local.
	KittySharedMemory bool
	Sixel             bool
	Name              string
	CellWidth         int
	CellHeight        int
}

// NativePosters requires Unicode placeholders, which basic Kitty graphics
// support alone does not guarantee (notably on older Kitty and other emulators).
func (c Capabilities) NativePosters() bool {
	if !c.Kitty {
		return false
	}
	name := strings.ToLower(c.Name)
	if strings.HasPrefix(name, "ghostty") {
		return true
	}
	if name == "" && (strings.EqualFold(os.Getenv("TERM_PROGRAM"), "ghostty") || os.Getenv("TERM") == "xterm-ghostty") {
		return true
	}
	if !strings.HasPrefix(name, "kitty(") {
		return false
	}
	version := strings.Split(strings.TrimSuffix(strings.TrimPrefix(c.Name, "kitty("), ")"), ".")
	if len(version) < 2 {
		return false
	}
	major, e1 := strconv.Atoi(version[0])
	minor, e2 := strconv.Atoi(version[1])
	return e1 == nil && e2 == nil && (major > 0 || major == 0 && minor >= 28)
}

var detected struct {
	sync.RWMutex
	caps Capabilities
}

var tmuxTerminal struct {
	sync.Once
	active bool
}

// InTmux checks the pane's actual TTY. A new Kitty window launched from a tmux
// shell inherits TMUX and TMUX_PANE but must receive unwrapped graphics.
func InTmux() bool {
	if os.Getenv("TMUX") == "" {
		return false
	}
	tmuxTerminal.Do(func() {
		pane := os.Getenv("TMUX_PANE")
		if pane == "" {
			return
		}
		ctx, cancel := context.WithTimeout(context.Background(), time.Second)
		defer cancel()
		cmd := exec.CommandContext(ctx, "tmux", "display-message", "-p", "-t", pane, "#{pane_tty}")
		cmd.WaitDelay = time.Second
		tty, err := cmd.Output()
		if err == nil {
			tmuxTerminal.active = sameTerminal(strings.TrimSpace(string(tty)), os.Stdin, os.Stdout)
		}
	})
	return tmuxTerminal.active
}

func sameTerminal(path string, streams ...*os.File) bool {
	pane, err := os.Stat(path)
	if err != nil {
		return false
	}
	for _, stream := range streams {
		if info, err := stream.Stat(); err == nil && os.SameFile(pane, info) {
			return true
		}
	}
	return false
}

func Current() Capabilities {
	detected.RLock()
	defer detected.RUnlock()
	return detected.caps
}

// EnablePassthrough changes only the current pane, and returns a restoration
// function. An inherited option is restored by removing the local override.
func EnablePassthrough() (func(), error) {
	noop := func() {}
	if !InTmux() {
		return noop, nil
	}
	pane := os.Getenv("TMUX_PANE")
	if pane == "" {
		return noop, fmt.Errorf("TMUX_PANE is missing; run plexctl inside a tmux pane")
	}
	run := func(args ...string) ([]byte, error) {
		ctx, cancel := context.WithTimeout(context.Background(), time.Second)
		defer cancel()
		cmd := exec.CommandContext(ctx, "tmux", args...)
		cmd.WaitDelay = time.Second
		return cmd.Output()
	}
	old, err := run("show-options", "-p", "-v", "-t", pane, "allow-passthrough")
	if err != nil {
		return noop, fmt.Errorf("tmux graphics require tmux 3.3 or newer: %w", err)
	}
	value := strings.TrimSpace(string(old))
	if value == "on" || value == "all" {
		return noop, nil
	}
	if _, err := run("set-option", "-p", "-t", pane, "allow-passthrough", "on"); err != nil {
		return noop, fmt.Errorf("enable tmux graphics passthrough: %w", err)
	}
	return func() {
		if value == "" {
			_, _ = run("set-option", "-p", "-u", "-t", pane, "allow-passthrough")
		} else {
			_, _ = run("set-option", "-p", "-t", pane, "allow-passthrough", value)
		}
	}, nil
}

func Wrap(sequence string) string {
	if InTmux() {
		return ansi.TmuxPassthrough(sequence)
	}
	return sequence
}

var attributes = regexp.MustCompile(`\x1b\[\?([0-9;]+)c`)
var terminalName = regexp.MustCompile(`\x1bP>\|([^\x1b]+)\x1b\\`)
var cellSizeReply = regexp.MustCompile(`\x1b\[6;(\d+);(\d+)t`)

func parseCapabilities(reply string) Capabilities {
	c := Capabilities{
		Kitty:             strings.Contains(reply, "_Gi=18901;OK"),
		KittySharedMemory: strings.Contains(reply, "_Gi=18902;OK"),
	}
	c.Kitty = c.Kitty || c.KittySharedMemory
	if match := attributes.FindStringSubmatch(reply); match != nil {
		c.Responded = true
		for _, feature := range strings.Split(match[1], ";")[1:] {
			c.Sixel = c.Sixel || feature == "4"
		}
	}
	if match := terminalName.FindStringSubmatch(reply); match != nil {
		c.Name = match[1]
		c.Responded = true
	}
	c.Responded = c.Responded || c.Kitty
	if match := cellSizeReply.FindStringSubmatch(reply); match != nil {
		c.CellHeight, _ = strconv.Atoi(match[1])
		c.CellWidth, _ = strconv.Atoi(match[2])
	}
	return c
}

// sharedMemoryQuery verifies that the terminal can actually read a random
// local memory object. SSH sessions fall back to escape-code transport.
// Linux exposes POSIX shared memory through /dev/shm; other platforms use the
// portable transport until they have a corresponding native probe.
func sharedMemoryQuery() (string, func()) {
	noop := func() {}
	if runtime.GOOS != "linux" {
		return "", noop
	}
	f, err := os.CreateTemp("/dev/shm", "plexctl-video-probe-")
	if err != nil {
		return "", noop
	}
	cleanup := func() { _ = os.Remove(f.Name()) }
	_, writeErr := f.Write([]byte{0, 0, 0})
	closeErr := f.Close()
	if writeErr != nil || closeErr != nil {
		cleanup()
		return "", noop
	}
	name := base64.StdEncoding.EncodeToString([]byte("/" + filepath.Base(f.Name())))
	return "\x1b_Ga=q,t=s,i=18902,s=1,v=1,f=24;" + name + "\x1b\\", cleanup
}

// Probe must run while the caller owns terminal input (before Bubble Tea starts
// or while it is suspended). A missing response is unknown, not rejection.
func Probe(input *os.File, output io.Writer) Capabilities {
	if !term.IsTerminal(int(input.Fd())) {
		return Current()
	}
	restore, err := EnablePassthrough()
	if err != nil {
		return Current()
	}
	defer restore()
	state, err := term.MakeRaw(int(input.Fd()))
	if err != nil {
		return Current()
	}
	defer func() {
		if err := term.Restore(int(input.Fd()), state); err != nil {
			slog.Warn("Restore terminal after graphics probe", "error", err)
		}
	}()
	r, err := cancelreader.NewReader(input)
	if err != nil {
		return Current()
	}
	var reply strings.Builder
	done := make(chan struct{})
	go func() {
		defer close(done)
		buf := make([]byte, 1024)
		for reply.Len() < 8192 {
			n, err := r.Read(buf)
			reply.Write(buf[:n])
			if err != nil {
				return
			}
		}
	}()
	shm, cleanup := sharedMemoryQuery()
	defer cleanup()
	_, _ = io.WriteString(output, Wrap("\x1b_Ga=q,U=1,i=18901,s=1,v=1,f=24;AAAA\x1b\\"+shm+"\x1b[c\x1b[>q\x1b[16t"))
	timer := time.NewTimer(350 * time.Millisecond)
	select {
	case <-done:
	case <-timer.C:
	}
	timer.Stop()
	r.Cancel()
	<-done
	_ = r.Close()
	caps := parseCapabilities(reply.String())
	detected.Lock()
	detected.caps = caps
	detected.Unlock()
	return caps
}
