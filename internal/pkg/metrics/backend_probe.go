package mintmakermetrics

import (
	"context"
	"sync/atomic"
)

type BackendProbe struct {
	scheduledRuns atomic.Int32
}

func NewBackendProbe() AvailabilityProbe {
	return &BackendProbe{
		scheduledRuns: atomic.Int32{},
	}
}

func (q *BackendProbe) CheckEvents(_ context.Context) float64 {
	defer q.scheduledRuns.Store(0)
	return float64(q.scheduledRuns.Load())
}

func (q *BackendProbe) AddEvent() {
	q.scheduledRuns.Add(1)
}
