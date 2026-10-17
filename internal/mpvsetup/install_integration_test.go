//go:build integration && linux

package mpvsetup

import (
	"bytes"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"golang.org/x/sys/unix"
)

func approvalTerminal(t *testing.T, answer string) *os.File {
	t.Helper()
	master, err := os.OpenFile("/dev/ptmx", os.O_RDWR|unix.O_NOCTTY, 0)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = master.Close() })
	if err := unix.IoctlSetPointerInt(int(master.Fd()), unix.TIOCSPTLCK, 0); err != nil {
		t.Fatal(err)
	}
	n, err := unix.IoctlGetInt(int(master.Fd()), unix.TIOCGPTN)
	if err != nil {
		t.Fatal(err)
	}
	path := fmt.Sprintf("/dev/pts/%d", n)
	slave, err := os.OpenFile(path, os.O_RDWR|unix.O_NOCTTY, 0)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = slave.Close() })
	if _, err := master.WriteString(answer); err != nil {
		t.Fatal(err)
	}
	return slave
}

func TestEnsureApprovalAndVerification(t *testing.T) {
	for _, tt := range []struct {
		name, initial, answer, result     string
		piped, required, wantRun, wantErr bool
	}{
		{name: "missing declined", answer: "n\n", required: true, wantErr: true},
		{name: "browse without mpv", answer: "\n"},
		{name: "outdated declined", initial: "0.37.0", answer: "n\n", required: true},
		{name: "piped yes rejected", answer: "yes\n", piped: true, required: true, wantErr: true},
		{name: "approved install", answer: "yes\n", result: "0.41.0", required: true, wantRun: true},
		{name: "approved update", initial: "0.37.0", answer: "y\n", result: "0.41.0", required: true, wantRun: true},
		{name: "repository still old", initial: "0.37.0", answer: "yes\n", result: "0.37.0", required: true, wantRun: true, wantErr: true},
		{name: "manager failure", answer: "yes\n", result: "fail", required: true, wantRun: true, wantErr: true},
	} {
		t.Run(tt.name, func(t *testing.T) {
			dir := t.TempDir()
			t.Setenv("PATH", dir)
			t.Setenv("MPV_TEST_DIR", dir)
			t.Setenv("MPV_TEST_RESULT", tt.result)
			write := func(name, body string) {
				t.Helper()
				if err := os.WriteFile(filepath.Join(dir, name), []byte(body), 0o700); err != nil {
					t.Fatal(err)
				}
			}
			if tt.initial != "" {
				write("mpv", "#!/bin/sh\nprintf 'mpv "+tt.initial+"\\n'\n")
			}
			// Only these inert scripts can be found on PATH. No system package
			// manager is invoked, even when exercising affirmative approval.
			write("brew", `#!/bin/sh
printf '%s\n' "$*" >> "$MPV_TEST_DIR/commands"
if [ "$MPV_TEST_RESULT" = fail ]; then exit 9; fi
if [ "$1" != update ]; then
  printf '#!/bin/sh\nprintf "mpv %s\\n"\n' "$MPV_TEST_RESULT" > "$MPV_TEST_DIR/mpv"
  /bin/chmod 700 "$MPV_TEST_DIR/mpv"
fi
`)
			cache, err := json.Marshal(releaseCache{Version: "0.41.0", Checked: time.Now()})
			if err != nil {
				t.Fatal(err)
			}
			if err := os.WriteFile(filepath.Join(dir, "mpv-release.json"), cache, 0o600); err != nil {
				t.Fatal(err)
			}
			var out bytes.Buffer
			opts := Options{CacheDir: dir, Output: &out, Error: &out, Required: tt.required, Install: true}
			if tt.piped {
				opts.Input = strings.NewReader(tt.answer)
			} else {
				opts.Input = approvalTerminal(t, tt.answer)
			}
			err = Ensure(t.Context(), opts)
			if (err != nil) != tt.wantErr {
				t.Fatalf("Ensure error=%v; output=%s", err, out.String())
			}
			commands, _ := os.ReadFile(filepath.Join(dir, "commands"))
			if (len(commands) > 0) != tt.wantRun {
				t.Fatalf("unexpected package operations: %s", commands)
			}
			if tt.wantRun && !strings.Contains(out.String(), "Proceed with these commands? [y/N]") {
				t.Fatal("installation ran without showing approval prompt")
			}
			if tt.name == "repository still old" && !strings.Contains(err.Error(), "still older") {
				t.Fatalf("outdated installation was not reported: %v", err)
			}
		})
	}
}
