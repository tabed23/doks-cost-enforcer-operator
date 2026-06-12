package controller

import "time"
const (
	pollInterval   = 5 * time.Minute
	digestInterval = 24 * time.Hour
	hoursPerMonth  = 730.0
	warnThreshold  = 0.80
	critThreshold  = 1.00

	alertLevelNone     = ""
	alertLevelWarning  = "warning"
	alertLevelCritical = "critical"

	conditionReady = "Ready"
)