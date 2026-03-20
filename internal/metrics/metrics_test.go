package mintmakermetrics

import (
	"context"
	"testing"
)

func TestBackendProbe(t *testing.T) {
	probe := NewBackendProbe()

	// Initially should return 0
	ctx := context.Background()
	events := probe.CheckEvents(ctx)
	if events != 0 {
		t.Errorf("expected 0 initial events, got %f", events)
	}

	// Add events and check
	probe.AddEvent()
	probe.AddEvent()
	probe.AddEvent()

	events = probe.CheckEvents(ctx)
	if events != 3 {
		t.Errorf("expected 3 events, got %f", events)
	}

	// CheckEvents should reset the counter
	events = probe.CheckEvents(ctx)
	if events != 0 {
		t.Errorf("expected 0 events after reset, got %f", events)
	}
}

func TestCountScheduledRunSuccess(t *testing.T) {
	// Reset global state
	probeSuccess = nil

	CountScheduledRunSuccess()
	CountScheduledRunSuccess()

	// Verify probe was created and events counted
	if probeSuccess == nil {
		t.Fatal("expected probeSuccess to be initialized")
	}
	events := (*probeSuccess).CheckEvents(context.Background())
	if events != 2 {
		t.Errorf("expected 2 success events, got %f", events)
	}
}

func TestCountScheduledRunFailure(t *testing.T) {
	// Reset global state
	probeFailure = nil

	CountScheduledRunFailure()

	if probeFailure == nil {
		t.Fatal("expected probeFailure to be initialized")
	}
	events := (*probeFailure).CheckEvents(context.Background())
	if events != 1 {
		t.Errorf("expected 1 failure event, got %f", events)
	}
}
