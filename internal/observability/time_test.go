package observability

import "time"

func timeForTest() time.Time {
	return time.Unix(0, 0).UTC()
}
