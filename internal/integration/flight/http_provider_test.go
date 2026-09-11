package flight

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

func TestHTTPJSONProviderMapsAODBEnvelopeAndAuth(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		if request.Header.Get("X-API-Key") != "test-key" {
			t.Fatalf("X-API-Key = %q, want test-key", request.Header.Get("X-API-Key"))
		}
		if request.URL.Query().Get("from") == "" || request.URL.Query().Get("to") == "" {
			t.Fatalf("expected polling window query parameters, got %s", request.URL.RawQuery)
		}
		writer.Header().Set("Content-Type", "application/json")
		if request.URL.Path == "/schedules" {
			_, _ = writer.Write([]byte(`{"data":{"items":[{"flight":{"id":"EXT-001","number":"CA123"},"ops":{"date":"2026-09-07","scheduled":"2026/09/07 10:30:00"}}]}}`))
			return
		}
		if request.URL.Path == "/events" {
			_, _ = writer.Write([]byte(`{"data":{"items":[{"eventId":"EV-001","flightId":"EXT-001","state":"ARR","time":"2026-09-07T10:45:00Z","reason":"on stand"}]}}`))
			return
		}
		http.NotFound(writer, request)
	}))
	defer server.Close()

	provider, err := NewHTTPJSONProvider(HTTPJSONProviderConfig{
		Name:          "aodb",
		BaseURL:       server.URL,
		SchedulesPath: "/schedules",
		EventsPath:    "/events",
		Timeout:       time.Second,
		Auth:          HTTPJSONAuthConfig{Type: "api_key", Token: "test-key", APIKeyHeader: "X-API-Key"},
		ScheduleMapping: ScheduleMapping{
			RecordsPath:      "data.items",
			ExternalFlightID: "flight.id",
			FlightDisplayNo:  "flight.number",
			OperatingDate:    "ops.date",
			ScheduledAt:      "ops.scheduled",
		},
		EventMapping: EventMapping{
			RecordsPath:      "data.items",
			ExternalEventID:  "eventId",
			ExternalFlightID: "flightId",
			Status:           "state",
			OccurredAt:       "time",
			Reason:           "reason",
			StatusMap:        map[string]string{"ARR": "arrived"},
		},
		TimeFormats: []string{"2006/01/02 15:04:05"},
	}, nil)
	if err != nil {
		t.Fatalf("NewHTTPJSONProvider() error = %v", err)
	}
	from := time.Date(2026, 9, 7, 0, 0, 0, 0, time.UTC)
	to := from.Add(24 * time.Hour)
	schedules, err := provider.ListSchedules(context.Background(), from, to)
	if err != nil {
		t.Fatalf("ListSchedules() error = %v", err)
	}
	if len(schedules) != 1 || schedules[0].Provider != "aodb" || schedules[0].ExternalFlightID != "EXT-001" || schedules[0].ScheduledAt.Hour() != 10 {
		t.Fatalf("unexpected schedules: %+v", schedules)
	}
	events, err := provider.ListEvents(context.Background(), from, to)
	if err != nil {
		t.Fatalf("ListEvents() error = %v", err)
	}
	if len(events) != 1 || events[0].Status != "arrived" || events[0].ExternalEventID != "EV-001" || events[0].Reason != "on stand" {
		t.Fatalf("unexpected events: %+v", events)
	}
}

func TestHTTPJSONProviderRejectsUnsupportedStatus(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		_, _ = writer.Write([]byte(`{"items":[{"id":"EV-001","flight":"EXT-001","status":"unknown","occurred":"2026-09-07T10:45:00Z"}]}`))
	}))
	defer server.Close()
	provider, err := NewHTTPJSONProvider(HTTPJSONProviderConfig{
		Name: "aodb", BaseURL: server.URL, SchedulesPath: "/schedules", EventsPath: "/events", Timeout: time.Second,
		ScheduleMapping: ScheduleMapping{RecordsPath: "items", ExternalFlightID: "id", FlightDisplayNo: "id", OperatingDate: "id", ScheduledAt: "id"},
		EventMapping:    EventMapping{RecordsPath: "items", ExternalEventID: "id", ExternalFlightID: "flight", Status: "status", OccurredAt: "occurred"},
	}, nil)
	if err != nil {
		t.Fatalf("NewHTTPJSONProvider() error = %v", err)
	}
	_, err = provider.ListEvents(context.Background(), time.Now().Add(-time.Hour), time.Now())
	if err == nil {
		t.Fatal("ListEvents() error = nil, want unsupported status error")
	}
}
