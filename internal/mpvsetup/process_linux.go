package mpvsetup

import (
	"context"
	"os"
	"os/exec"
	"os/signal"
	"syscall"
	"time"
)

func buildContext(ctx context.Context) (context.Context, context.CancelFunc) {
	return signal.NotifyContext(ctx, os.Interrupt, syscall.SIGTERM)
}

func prepareBuildCommand(cmd *exec.Cmd) {
	cmd.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}
	// Meson spawns Ninja and compilers; cancel the entire build group so they
	// cannot keep writing into a prefix that is being cleaned up.
	cmd.Cancel = func() error { return syscall.Kill(-cmd.Process.Pid, syscall.SIGKILL) }
	cmd.WaitDelay = time.Second
}
