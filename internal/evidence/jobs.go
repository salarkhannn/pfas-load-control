package evidence

import "github.com/riverqueue/river"

const Queue = "physical-evidence"

type EvaluateArgs struct {
	EvaluationID string `json:"evaluationId"`
}

func (EvaluateArgs) Kind() string { return "evaluate_field_physical_evidence" }

func (args EvaluateArgs) InsertOpts() river.InsertOpts {
	return river.InsertOpts{
		MaxAttempts: 1,
		Queue:       Queue,
		UniqueOpts: river.UniqueOpts{
			ByArgs: true,
		},
	}
}
