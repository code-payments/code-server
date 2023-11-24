package metrics

import (
	"context"
	"fmt"

	"github.com/newrelic/go-agent/v3/newrelic"
)

// TraceMethodCall traces a method call with a given struct/package and method names
func TraceMethodCall(ctx context.Context, structOrPackageName, methodName string) *MethodTracer {
	txn := newrelic.FromContext(ctx)
	if txn == nil {
		return nil
	}

	seg := txn.StartSegment(fmt.Sprintf("%s %s", structOrPackageName, methodName))

	return &MethodTracer{
		txn: txn,
		seg: seg,
	}
}

// MethodTracer collects analytics for a given method call within an existing
// trace.
type MethodTracer struct {
	txn *newrelic.Transaction
	seg *newrelic.Segment
}

// AddAttribute adds a key-value pair metadata to the method trace
func (t *MethodTracer) AddAttribute(key string, value interface{}) {
	if t == nil {
		return
	}

	t.seg.AddAttribute(key, value)
}

// AddAttributes adds a set of key-value pair metadata to the method trace
func (t *MethodTracer) AddAttributes(attributes map[string]interface{}) {
	if t == nil {
		return
	}

	for key, value := range attributes {
		t.seg.AddAttribute(key, value)
	}
}

// OnError observes an error within a method trace
func (t *MethodTracer) OnError(err error) {
	if t == nil {
		return
	}

	if err == nil {
		return
	}

	t.txn.NoticeError(err)
}

// End completes the trace for the method call.
func (t *MethodTracer) End() {
	if t == nil {
		return
	}

	t.seg.End()
}
