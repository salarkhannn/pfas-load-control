package agent

import "github.com/riverqueue/river"

const readinessQueue = "readiness"

type AdvanceReadinessArgs struct {
	RunID string `json:"runId"`
	Step  int16  `json:"step"`
}

func (AdvanceReadinessArgs) Kind() string { return "advance_mireye_readiness" }

func (AdvanceReadinessArgs) InsertOpts() river.InsertOpts {
	return river.InsertOpts{
		MaxAttempts: 3,
		Queue:       readinessQueue,
		UniqueOpts: river.UniqueOpts{
			ByArgs: true,
		},
	}
}
