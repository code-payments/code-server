package metrics

import (
	"context"

	"github.com/newrelic/go-agent/v3/newrelic"
)

// RecordEvent records a new event with a name and set of key-value pairs
func RecordEvent(ctx context.Context, eventName string, kvPairs map[string]interface{}) {
	nr, ok := ctx.Value(NewRelicContextKey).(*newrelic.Application)
	if ok {
		nr.RecordCustomEvent(eventName, kvPairs)
	}
}
