package player

import (
	_ "embed"
	"os"
	"path/filepath"
)

const WaveformSamples = 256

// Waveform is a live stereo trace from the playing decoder's showwaves output.
// Fixed arrays bound IPC/rendering memory and make player status safe to copy.
type Waveform struct {
	Left   [WaveformSamples]float64 `json:"left"`
	Right  [WaveformSamples]float64 `json:"right"`
	Serial int64                    `json:"serial"`
	Valid  bool                     `json:"valid"`
}

//go:embed waveform.lua
var waveformScript []byte

// Called with ioMu held. Lua can read mpv's raw video frame in memory; JSON IPC
// cannot carry that binary node. Only the small stereo trace crosses IPC.
func (pm *PlayerManager) ensureWaveformScript() error {
	var version int
	if pm.conn.get("user-data/plexctl-waveform-version", &version) == nil && version == 1 {
		return nil
	}
	if pm.waveformDir == "" {
		dir, err := os.MkdirTemp("", "plexctl-waveform-")
		if err != nil {
			return err
		}
		pm.waveformDir = dir
	}
	path := filepath.Join(pm.waveformDir, "plexctl-waveform.lua")
	if err := os.WriteFile(path, waveformScript, 0o600); err != nil {
		return err
	}
	return pm.conn.call("load-script", path)
}
