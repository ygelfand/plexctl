//go:build !linux

package mpvsetup

import (
	"context"
	"os"
	"os/exec"
	"os/signal"
)

func buildContext(ctx context.Context) (context.Context, context.CancelFunc) {
	return signal.NotifyContext(ctx, os.Interrupt)
}

func prepareBuildCommand(_ *exec.Cmd) {}
