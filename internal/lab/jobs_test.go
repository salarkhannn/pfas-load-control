package lab

import (
	"testing"
)

func TestIngestJobCanBeQueuedAgainAfterCompletion(t *testing.T) {
	unique := (IngestArgs{}).InsertOpts().UniqueOpts
	if unique.ByArgs || unique.ByPeriod != 0 || unique.ByQueue || unique.ByState != nil || unique.ExcludeKind {
		t.Fatal("queue uniqueness prevents a failed report from being retried; report state already prevents duplicate enqueueing")
	}
}
