package mintmakermetrics

import (
	"context"
	"fmt"
	"time"

	"github.com/go-logr/logr"
	"github.com/prometheus/client_golang/prometheus"
)

var (
	probeSuccess              *AvailabilityProbe
	probeFailure              *AvailabilityProbe
	controllerAvailabilityVec = prometheus.NewGaugeVec(
		prometheus.GaugeOpts{
			Subsystem: "mintmaker",
			Name:      "availability",
			Help:      "Number of successfully scheduled MintMaker PipelineRuns and failures",
		},
		[]string{"status"}, // "success" or "failure"
	)
)

func RegisterCommonMetrics(ctx context.Context, registerer prometheus.Registerer) error {
	log := logr.FromContextOrDiscard(ctx)
	if err := registerer.Register(controllerAvailabilityVec); err != nil {
		return fmt.Errorf("failed to register metrics: %w", err)
	}
	ticker := time.NewTicker(10 * time.Minute)
	log.Info("Starting metrics")
	go func() {
		for {
			select {
			case <-ctx.Done(): // Shutdown if context is canceled
				log.Info("Shutting down metrics")
				ticker.Stop()
				return
			case <-ticker.C:
				checkProbes(ctx)
			}
		}
	}()
	return nil
}

func checkProbes(ctx context.Context) {
	// Set availability metric based on contoller events (scheduled PipelineRuns)
	if probeSuccess != nil {
		successEvents := (*probeSuccess).CheckEvents(ctx)
		controllerAvailabilityVec.WithLabelValues("success").Set(successEvents)
	}
	if probeFailure != nil {
		failureEvents := (*probeFailure).CheckEvents(ctx)
		controllerAvailabilityVec.WithLabelValues("failure").Set(failureEvents)
	}

}

func CountScheduledRunSuccess() {
	if probeSuccess == nil {
		watcher := NewBackendProbe()
		probeSuccess = &watcher
	}
	(*probeSuccess).AddEvent()
}

func CountScheduledRunFailure() {
	if probeFailure == nil {
		watcher := NewBackendProbe()
		probeFailure = &watcher
	}
	(*probeFailure).AddEvent()
}

type AvailabilityProbe interface {
	CheckEvents(ctx context.Context) float64
	AddEvent()
}
