package store

import (
	"errors"

	"dnsmonitor/internal/model"
)

var ErrTrustedOverride = errors.New("可信 DNS 不允许手动定罪；请先取消可信标记")

// Trust is a current user policy, separate from immutable historical probe evidence.
// Apply it before scoring so removing an old F grade restores the computed quality score.
func (a *accumulator) finishForServer(window int64, trusted bool) model.Metrics {
	if trusted {
		copy := *a
		copy.pollution = 1
		return copy.finish(window)
	}
	return a.finish(window)
}
