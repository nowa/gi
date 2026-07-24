package gillmprovider

import (
	"errors"
	"math"
	"time"
)

var errOAuthExpiryOutOfRange = errors.New("OAuth token expiry is out of range")

// oauthExpiryMillis converts a provider lifetime into the credential store's
// Unix-millisecond representation without relying on time.Duration, whose
// roughly 290-year range is smaller than time.Time's supported range.
func oauthExpiryMillis(
	now time.Time,
	expiresInSeconds float64,
	skew time.Duration,
) (int64, error) {
	const (
		maxInt64Exclusive = float64(1 << 63)
		maxInt64          = int64(^uint64(0) >> 1)
		minInt64          = -maxInt64 - 1
	)
	lifetimeMillis := expiresInSeconds * float64(time.Second/time.Millisecond)
	if math.IsNaN(expiresInSeconds) ||
		math.IsInf(expiresInSeconds, 0) ||
		expiresInSeconds <= 0 ||
		math.IsInf(lifetimeMillis, 0) ||
		lifetimeMillis >= maxInt64Exclusive {
		return 0, errOAuthExpiryOutOfRange
	}
	delta := int64(lifetimeMillis) - skew.Milliseconds()
	nowMillis := now.UnixMilli()
	if (delta > 0 && nowMillis > maxInt64-delta) ||
		(delta < 0 && nowMillis < minInt64-delta) {
		return 0, errOAuthExpiryOutOfRange
	}
	return nowMillis + delta, nil
}
