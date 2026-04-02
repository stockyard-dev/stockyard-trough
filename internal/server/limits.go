package server

import "github.com/stockyard-dev/stockyard-trough/internal/license"

// Limits holds the feature limits for the current license tier.
// All int limits: 0 means unlimited (Pro tier only).
type Limits struct {
	MaxUpstreams int // 0 = unlimited (Pro)
	MaxRequestsPerMonth int // 0 = unlimited (Pro)
	HistoryDays int // 0 = unlimited (Pro)
	AnomalyDetection bool
	SpendAlerts bool
	SpendTrends bool
}

var freeLimits = Limits{
		MaxUpstreams: 1,
		MaxRequestsPerMonth: 10000,
		HistoryDays: 7,
		AnomalyDetection: false,
		SpendAlerts: false,
		SpendTrends: false,
}

var proLimits = Limits{
		MaxUpstreams: 0,
		MaxRequestsPerMonth: 0,
		HistoryDays: 90,
		AnomalyDetection: true,
		SpendAlerts: true,
		SpendTrends: true,
}

// LimitsFor returns the appropriate Limits for the given license info.
// nil info = no key set = free tier.
func LimitsFor(info *license.Info) Limits {
	if info != nil && info.IsPro() {
		return proLimits
	}
	return freeLimits
}

// LimitReached returns true if the current count meets or exceeds the limit.
// A limit of 0 is treated as unlimited.
func LimitReached(limit, current int) bool {
	if limit == 0 {
		return false
	}
	return current >= limit
}
