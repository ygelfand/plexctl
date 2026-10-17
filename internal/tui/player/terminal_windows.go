package player

import (
	"net"
	"os"
	"time"

	"gopkg.in/natefinch/npipe.v2"
)

func dialTerminal(socket string) (net.Conn, error) {
	return npipe.DialTimeout(socket, 100*time.Millisecond)
}

// Windows terminals provide pixel metrics through the terminal cell-size reply.
func terminalCellSize(*os.File) (float64, float64) { return 0, 0 }
