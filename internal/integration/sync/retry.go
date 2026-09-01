package sync

import "time"

type RetryPolicy struct {
	MaxAttempts int
	BaseDelay   time.Duration
}

func (p RetryPolicy) Next(attempt int, now time.Time) (time.Time, bool) {
	max := p.MaxAttempts
	if max <= 0 {
		max = 5
	}
	if attempt >= max {
		return time.Time{}, false
	}
	base := p.BaseDelay
	if base <= 0 {
		base = time.Second
	}
	delay := base
	for i := 1; i < attempt; i++ {
		if delay >= time.Hour {
			break
		}
		delay *= 2
	}
	return now.UTC().Add(delay), true
}
