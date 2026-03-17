package api

import "time"

func secondsDuration(secs int64) time.Duration {
	return time.Duration(secs) * time.Second
}
