package mmmetrics

import (
	"context"
	"fmt"
	"time"

	"github.com/go-logr/logr"
	"github.com/prometheus/client_golang/prometheus"
)

var (
	probe                       *AvailabilityProbe
	controllerAvailabilityGauge = prometheus.NewGauge(
		prometheus.GaugeOpts{
			Subsystem: "mintmaker",
			Name:      "available",
			Help:      "Number of scheduled MintMaker jobs",
		})
)

func RegisterCommonMetrics(ctx context.Context, registerer prometheus.Registerer) error {
	log := logr.FromContextOrDiscard(ctx)
	if err := registerer.Register(controllerAvailabilityGauge); err != nil {
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
	events := (*probe).CheckEvents(ctx)
	controllerAvailabilityGauge.Set(events)
}

func CountScheduledRuns() {
	if probe == nil {
		watcher := NewBackendProbe()
		probe = &watcher
	}
	(*probe).AddEvent()
}

type AvailabilityProbe interface {
	CheckEvents(ctx context.Context) float64
	AddEvent()
}
