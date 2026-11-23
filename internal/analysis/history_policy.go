package analysis

import (
	"math"
	"time"
)

const (
	DefaultRetentionDays = 30
	MaxRetentionDays     = int(math.MaxInt64 / (24 * time.Hour))
)
