package quota

import (
	"time"

	"github.com/llm-proxy/llm-proxy/internal/provider"
)

func Int64(v int64) *int64 { return &v }

func Clone(q *provider.Quota) *provider.Quota {
	if q == nil {
		return nil
	}
	cp := *q
	cp.RPM = copyInt64(q.RPM)
	cp.RPD = copyInt64(q.RPD)
	cp.TPM = copyInt64(q.TPM)
	cp.TPD = copyInt64(q.TPD)
	cp.RemainingRPM = copyInt64(q.RemainingRPM)
	cp.RemainingRPD = copyInt64(q.RemainingRPD)
	cp.UsedRPD = copyInt64(q.UsedRPD)
	return &cp
}

func copyInt64(v *int64) *int64 {
	if v == nil {
		return nil
	}
	x := *v
	return &x
}

func NextMidnightUTC(now time.Time) string {
	n := now.UTC()
	next := time.Date(n.Year(), n.Month(), n.Day()+1, 0, 0, 0, 0, time.UTC)
	return next.Format(time.RFC3339)
}

func NextMidnightPacific(now time.Time) string {
	loc, err := time.LoadLocation("America/Los_Angeles")
	if err != nil {
		return ""
	}
	n := now.In(loc)
	next := time.Date(n.Year(), n.Month(), n.Day()+1, 0, 0, 0, 0, loc)
	return next.Format(time.RFC3339)
}

// ApplyLiveRemaining fills remaining/used fields on a shared-pool quota.
func ApplyLiveRemaining(base *provider.Quota, remaining, used, limit *int64) *provider.Quota {
	q := Clone(base)
	if q == nil {
		q = &provider.Quota{}
	}
	if limit != nil {
		q.RPD = copyInt64(limit)
	}
	q.RemainingRPD = copyInt64(remaining)
	q.UsedRPD = copyInt64(used)
	q.Source = "live"
	q.Confidence = "exact"
	return q
}
