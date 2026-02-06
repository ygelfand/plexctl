package player

import "github.com/LukeHagar/plexgo/models/operations"

type PlayerStatus struct {
	Time     float64
	Duration float64
	Title    string
	File     string
	Key      string
	State    operations.State
	NoReport bool
	TctMode  bool
}

type (
	PlayerStatusMsg       struct{}
	PlayerStatusMsgPoller struct{}
)

func (ps *PlayerStatus) Paused() bool { return ps.State == operations.StatePaused }

func (ps *PlayerStatus) Active() bool {
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
