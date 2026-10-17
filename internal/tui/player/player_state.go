package player

import (
	"fmt"
	"github.com/LukeHagar/plexgo/models/operations"
	"github.com/ygelfand/plexctl/internal/config"
)

type VideoFallbackMsg struct{ From config.VideoOutput }

func (m VideoFallbackMsg) String() string {
	return fmt.Sprintf("%s graphics could not start; using the Text renderer. Select a renderer in Settings to override.", m.From)
}

type PlayerStatus struct {
	Server    config.ServerTarget
	Time      float64
	Duration  float64
	Title     string
	File      string
	Key       string
	State     operations.State
	NoReport  bool
	AudioOnly bool
	Overlay   bool
	Volume    float64
	Muted     bool
	Spectrum  Spectrum
	Waveform  Waveform
}

type ExternalSubtitle struct {
	URL      string
	Title    string
	Language string
}

type (
	PlayerStatusMsg       struct{}
	PlayerStatusMsgPoller struct{}
)

func (ps PlayerStatus) Paused() bool { return ps.State == operations.StatePaused }

func (ps PlayerStatus) Active() bool {
	return ps.State != "" && ps.State != operations.StateStopped
}

func (ps *PlayerStatus) Play() {
	ps.State = operations.StatePlaying
}

func (ps *PlayerStatus) Pause() {
	if ps.State == operations.StatePlaying {
		ps.State = operations.StatePaused
	}
}
