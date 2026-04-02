package server

type Limits struct {
	MaxUpstreams int // 0 = unlimited (Pro)
	MaxRequestsPerMonth int // 0 = unlimited (Pro)
	HistoryDays int // 0 = unlimited (Pro)
	AnomalyDetection bool
	SpendAlerts bool
	SpendTrends bool
}

// DefaultLimits returns fully-unlocked limits for the standalone edition.
func DefaultLimits() Limits {
	return Limits{
		MaxUpstreams: 0,
		MaxRequestsPerMonth: 0,
		HistoryDays: 90,
		AnomalyDetection: true,
		SpendAlerts: true,
		SpendTrends: true,
}
}

// LimitReached returns true if the current count meets or exceeds the limit.
// A limit of 0 is treated as unlimited.
func LimitReached(limit, current int) bool {
	if limit == 0 {
		return false
	}
	return current >= limit
}
