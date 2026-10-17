//go:build !windows

package player

import (
	"net"
	"os"
	"time"

	"golang.org/x/sys/unix"
)

func dialTerminal(socket string) (net.Conn, error) {
	return net.DialTimeout("unix", socket, 100*time.Millisecond)
}

func terminalCellSize(input *os.File) (float64, float64) {
	if size, err := unix.IoctlGetWinsize(int(input.Fd()), unix.TIOCGWINSZ); err == nil && size.Col > 0 && size.Row > 0 && size.Xpixel > 0 && size.Ypixel > 0 {
		return float64(size.Xpixel) / float64(size.Col), float64(size.Ypixel) / float64(size.Row)
	}
	return 0, 0
}
