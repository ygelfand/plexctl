//go:build integration && !windows

package mpvsetup

import (
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
	"time"
)

func TestInspectBoundsInheritedOutputPipe(t *testing.T) {
	dir := t.TempDir()
	path, pidPath := filepath.Join(dir, "mpv"), filepath.Join(dir, "child.pid")
	t.Setenv("MPV_TEST_CHILD_PID", pidPath)
	// The wrapper exits immediately, but its descendant retains stdout.
	script := "#!/bin/sh\n/bin/sleep 30 &\nprintf '%s' \"$!\" > \"$MPV_TEST_CHILD_PID\"\nprintf 'mpv 0.41.0\\n'\n"
	if err := os.WriteFile(path, []byte(script), 0o700); err != nil {
		t.Fatal(err)
	}
	done := make(chan error, 1)
	finished := make(chan struct{})
	t.Cleanup(func() {
		if data, err := os.ReadFile(pidPath); err == nil {
			if pid, err := strconv.Atoi(strings.TrimSpace(string(data))); err == nil {
				if child, err := os.FindProcess(pid); err == nil {
					_ = child.Kill()
				}
			}
		}
		<-finished
	})
	go func() {
		defer close(finished)
		_, err := inspect(t.Context(), path)
		done <- err
	}()
	select {
	case err := <-done:
		if !errors.Is(err, exec.ErrWaitDelay) {
			t.Fatalf("probe did not report its inherited pipe timeout: %v", err)
		}
	case <-time.After(3 * time.Second):
		t.Fatal("mpv version probe waited for a descendant's stdout")
	}
}
