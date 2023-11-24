package metrics

import (
	"context"
	"time"

	"github.com/newrelic/go-agent/v3/newrelic"
)

// RecordCount records a count metric
func RecordCount(ctx context.Context, metricName string, count uint64) {
	nr, ok := ctx.Value(NewRelicContextKey).(*newrelic.Application)
	if ok {
		nr.RecordCustomMetric(metricName, float64(count))
	}
}

// RecordDuration records a duration metric
func RecordDuration(ctx context.Context, metricName string, duration time.Duration) {
	nr, ok := ctx.Value(NewRelicContextKey).(*newrelic.Application)
	if ok {
		nr.RecordCustomMetric(metricName, float64(duration/time.Millisecond))
	}
}
