// Package flight contains the integration boundary for upstream flight
// systems. Core owns the normalized flight facts, while a provider owns the
// transport-specific API, authentication and field mapping.
package flight

import (
	"context"
	"time"
)

// Schedule is the normalized schedule record returned by an upstream flight
// provider. It is intentionally free of GORM and HTTP types so providers can
// be replaced without changing Core business code.
type Schedule struct {
	Provider         string    `json:"provider"`
	ExternalFlightID string    `json:"external_flight_id"`
	FlightDisplayNo  string    `json:"flight_display_no"`
	OperatingDate    time.Time `json:"operating_date"`
	ScheduledAt      time.Time `json:"scheduled_at"`
}

// Event is a normalized upstream lifecycle event. Arrival, departure,
// cancellation and delay facts come from the provider; management users do
// not create or advance flight facts from the browser.
type Event struct {
	Provider          string     `json:"provider"`
	ExternalEventID   string     `json:"external_event_id"`
	ExternalFlightID  string     `json:"external_flight_id"`
	Status            string     `json:"status"`
	OccurredAt        time.Time  `json:"occurred_at"`
	ActualArrivalAt   *time.Time `json:"actual_arrival_at,omitempty"`
	ActualDepartureAt *time.Time `json:"actual_departure_at,omitempty"`
	ScheduledAt       *time.Time `json:"scheduled_at,omitempty"`
	Reason            string     `json:"reason,omitempty"`
}

// Provider is the future adapter contract for the airline/AODB/airport
// source. The current repository uses development-seeded rows until a real
// provider is configured; no provider credentials belong in Core domain code.
type Provider interface {
	Name() string
	ListSchedules(context.Context, time.Time, time.Time) ([]Schedule, error)
	ListEvents(context.Context, time.Time, time.Time) ([]Event, error)
}
