package flight

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"regexp"
	"strconv"
	"strings"
	"time"
)

var providerNamePattern = regexp.MustCompile(`^[a-z][a-z0-9_.-]{1,63}$`)

// HTTPJSONProviderConfig describes the transport and mapping contract for a
// real AODB/airline HTTP endpoint. The application layer only sees the
// normalized Schedule/Event types from provider.go.
type HTTPJSONProviderConfig struct {
	Name            string
	BaseURL         string
	SchedulesPath   string
	EventsPath      string
	FromQuery       string
	ToQuery         string
	Timeout         time.Duration
	UserAgent       string
	Headers         map[string]string
	Auth            HTTPJSONAuthConfig
	ScheduleMapping ScheduleMapping
	EventMapping    EventMapping
	TimeFormats     []string
}

type HTTPJSONAuthConfig struct {
	Type         string
	Token        string
	APIKeyHeader string
	Username     string
	Password     string
}

type ScheduleMapping struct {
	RecordsPath      string
	ExternalFlightID string
	FlightDisplayNo  string
	OperatingDate    string
	ScheduledAt      string
}

type EventMapping struct {
	RecordsPath       string
	ExternalEventID   string
	ExternalFlightID  string
	Status            string
	OccurredAt        string
	ActualArrivalAt   string
	ActualDepartureAt string
	ScheduledAt       string
	Reason            string
	StatusMap         map[string]string
}

// HTTPJSONProvider is intentionally protocol-light: different AODB and
// airline APIs commonly differ in envelope shape, field names and status
// vocabulary. Those differences belong in the mapping configuration, not in
// Core domain code.
type HTTPJSONProvider struct {
	config HTTPJSONProviderConfig
	client *http.Client
	base   *url.URL
}

func NewHTTPJSONProvider(config HTTPJSONProviderConfig, client *http.Client) (*HTTPJSONProvider, error) {
	if err := config.Validate(); err != nil {
		return nil, err
	}
	base, err := url.Parse(strings.TrimSpace(config.BaseURL))
	if err != nil {
		return nil, fmt.Errorf("parse flight source base URL: %w", err)
	}
	if client == nil {
		client = &http.Client{Timeout: config.Timeout}
	}
	if client.Timeout <= 0 {
		client.Timeout = config.Timeout
	}
	if config.Headers == nil {
		config.Headers = map[string]string{}
	}
	if config.FromQuery == "" {
		config.FromQuery = "from"
	}
	if config.ToQuery == "" {
		config.ToQuery = "to"
	}
	if config.UserAgent == "" {
		config.UserAgent = "flight-collaboration-platform/flight-source"
	}
	return &HTTPJSONProvider{config: config, client: client, base: base}, nil
}

func (c HTTPJSONProviderConfig) Validate() error {
	if !providerNamePattern.MatchString(strings.TrimSpace(c.Name)) || strings.EqualFold(strings.TrimSpace(c.Name), "manual") {
		return errors.New("flight source provider name is invalid")
	}
	base, err := url.Parse(strings.TrimSpace(c.BaseURL))
	if err != nil || base.Scheme == "" || base.Host == "" || (base.Scheme != "http" && base.Scheme != "https") {
		return errors.New("flight source base_url must be an absolute http or https URL")
	}
	if strings.TrimSpace(c.SchedulesPath) == "" || strings.TrimSpace(c.EventsPath) == "" {
		return errors.New("flight source schedules_path and events_path are required")
	}
	if c.Timeout <= 0 {
		return errors.New("flight source timeout must be positive")
	}
	if strings.TrimSpace(c.ScheduleMapping.RecordsPath) == "" || strings.TrimSpace(c.EventMapping.RecordsPath) == "" {
		return errors.New("flight source records_path is required for schedules and events")
	}
	if err := c.ScheduleMapping.validate(); err != nil {
		return fmt.Errorf("schedule mapping: %w", err)
	}
	if err := c.EventMapping.validate(); err != nil {
		return fmt.Errorf("event mapping: %w", err)
	}
	authType := strings.ToLower(strings.TrimSpace(c.Auth.Type))
	switch authType {
	case "", "none":
	case "bearer":
		if strings.TrimSpace(c.Auth.Token) == "" {
			return errors.New("bearer auth token is required")
		}
	case "api_key":
		if strings.TrimSpace(c.Auth.Token) == "" || strings.TrimSpace(c.Auth.APIKeyHeader) == "" {
			return errors.New("api_key auth token and api_key_header are required")
		}
	case "basic":
		if strings.TrimSpace(c.Auth.Username) == "" || c.Auth.Password == "" {
			return errors.New("basic auth username and password are required")
		}
	default:
		return fmt.Errorf("unsupported flight source auth type %q", c.Auth.Type)
	}
	return nil
}

func (m ScheduleMapping) validate() error {
	for name, path := range map[string]string{
		"external_flight_id": m.ExternalFlightID,
		"flight_display_no":  m.FlightDisplayNo,
		"operating_date":     m.OperatingDate,
		"scheduled_at":       m.ScheduledAt,
	} {
		if strings.TrimSpace(path) == "" {
			return fmt.Errorf("%s path is required", name)
		}
	}
	return nil
}

func (m EventMapping) validate() error {
	for name, path := range map[string]string{
		"external_event_id":  m.ExternalEventID,
		"external_flight_id": m.ExternalFlightID,
		"status":             m.Status,
		"occurred_at":        m.OccurredAt,
	} {
		if strings.TrimSpace(path) == "" {
			return fmt.Errorf("%s path is required", name)
		}
	}
	return nil
}

func (p *HTTPJSONProvider) Name() string { return p.config.Name }

func (p *HTTPJSONProvider) ListSchedules(ctx context.Context, from, to time.Time) ([]Schedule, error) {
	rows, err := p.fetchRecords(ctx, p.config.SchedulesPath, p.config.ScheduleMapping.RecordsPath, from, to)
	if err != nil {
		return nil, fmt.Errorf("list schedules: %w", err)
	}
	result := make([]Schedule, 0, len(rows))
	for index, row := range rows {
		value, err := decodeSchedule(row, p.config.ScheduleMapping, p.config.TimeFormats)
		if err != nil {
			return nil, fmt.Errorf("decode schedule %d: %w", index, err)
		}
		value.Provider = p.Name()
		result = append(result, value)
	}
	return result, nil
}

func (p *HTTPJSONProvider) ListEvents(ctx context.Context, from, to time.Time) ([]Event, error) {
	rows, err := p.fetchRecords(ctx, p.config.EventsPath, p.config.EventMapping.RecordsPath, from, to)
	if err != nil {
		return nil, fmt.Errorf("list events: %w", err)
	}
	result := make([]Event, 0, len(rows))
	for index, row := range rows {
		value, err := decodeEvent(row, p.config.EventMapping, p.config.TimeFormats)
		if err != nil {
			return nil, fmt.Errorf("decode event %d: %w", index, err)
		}
		value.Provider = p.Name()
		result = append(result, value)
	}
	return result, nil
}

func (p *HTTPJSONProvider) fetchRecords(ctx context.Context, path, recordsPath string, from, to time.Time) ([]map[string]any, error) {
	if p == nil || p.client == nil || p.base == nil {
		return nil, errors.New("flight source provider is not configured")
	}
	if from.IsZero() || to.IsZero() || !to.After(from) {
		return nil, errors.New("flight source query window is invalid")
	}
	u := *p.base
	u.Path = strings.TrimRight(u.Path, "/") + "/" + strings.TrimLeft(path, "/")
	query := u.Query()
	query.Set(p.config.FromQuery, from.UTC().Format(time.RFC3339))
	query.Set(p.config.ToQuery, to.UTC().Format(time.RFC3339))
	u.RawQuery = query.Encode()
	request, err := http.NewRequestWithContext(ctx, http.MethodGet, u.String(), nil)
	if err != nil {
		return nil, fmt.Errorf("create flight source request: %w", err)
	}
	request.Header.Set("Accept", "application/json")
	request.Header.Set("User-Agent", p.config.UserAgent)
	for key, value := range p.config.Headers {
		request.Header.Set(key, value)
	}
	switch strings.ToLower(strings.TrimSpace(p.config.Auth.Type)) {
	case "bearer":
		request.Header.Set("Authorization", "Bearer "+p.config.Auth.Token)
	case "api_key":
		request.Header.Set(p.config.Auth.APIKeyHeader, p.config.Auth.Token)
	case "basic":
		request.SetBasicAuth(p.config.Auth.Username, p.config.Auth.Password)
	}
	response, err := p.client.Do(request)
	if err != nil {
		return nil, fmt.Errorf("request flight source: %w", err)
	}
	defer response.Body.Close()
	if response.StatusCode < http.StatusOK || response.StatusCode >= http.StatusMultipleChoices {
		body, _ := io.ReadAll(io.LimitReader(response.Body, 2048))
		return nil, fmt.Errorf("flight source returned HTTP %d: %s", response.StatusCode, strings.TrimSpace(string(body)))
	}
	var document any
	if err := json.NewDecoder(response.Body).Decode(&document); err != nil {
		return nil, fmt.Errorf("decode flight source JSON: %w", err)
	}
	value, err := lookupPath(document, recordsPath)
	if err != nil {
		return nil, fmt.Errorf("find records at %q: %w", recordsPath, err)
	}
	items, ok := value.([]any)
	if !ok {
		return nil, fmt.Errorf("records at %q must be an array", recordsPath)
	}
	result := make([]map[string]any, 0, len(items))
	for index, item := range items {
		row, ok := item.(map[string]any)
		if !ok {
			return nil, fmt.Errorf("record %d at %q must be an object", index, recordsPath)
		}
		result = append(result, row)
	}
	return result, nil
}

func decodeSchedule(row map[string]any, mapping ScheduleMapping, formats []string) (Schedule, error) {
	flightID, err := requiredString(row, mapping.ExternalFlightID)
	if err != nil {
		return Schedule{}, err
	}
	displayNo, err := requiredString(row, mapping.FlightDisplayNo)
	if err != nil {
		return Schedule{}, err
	}
	operatingDate, err := requiredTime(row, mapping.OperatingDate, formats)
	if err != nil {
		return Schedule{}, err
	}
	scheduledAt, err := requiredTime(row, mapping.ScheduledAt, formats)
	if err != nil {
		return Schedule{}, err
	}
	return Schedule{ExternalFlightID: flightID, FlightDisplayNo: displayNo, OperatingDate: operatingDate, ScheduledAt: scheduledAt}, nil
}

func decodeEvent(row map[string]any, mapping EventMapping, formats []string) (Event, error) {
	externalEventID, err := requiredString(row, mapping.ExternalEventID)
	if err != nil {
		return Event{}, err
	}
	externalFlightID, err := requiredString(row, mapping.ExternalFlightID)
	if err != nil {
		return Event{}, err
	}
	rawStatus, err := requiredString(row, mapping.Status)
	if err != nil {
		return Event{}, err
	}
	status := normalizeStatus(rawStatus, mapping.StatusMap)
	occurredAt, err := requiredTime(row, mapping.OccurredAt, formats)
	if err != nil {
		return Event{}, err
	}
	result := Event{ExternalEventID: externalEventID, ExternalFlightID: externalFlightID, Status: status, OccurredAt: occurredAt}
	if result.Status == "" {
		return Event{}, fmt.Errorf("unsupported source status %q", rawStatus)
	}
	for name, path := range map[string]string{
		"actual_arrival_at":   mapping.ActualArrivalAt,
		"actual_departure_at": mapping.ActualDepartureAt,
		"scheduled_at":        mapping.ScheduledAt,
	} {
		if strings.TrimSpace(path) == "" {
			continue
		}
		value, exists, lookupErr := optionalTime(row, path, formats)
		if lookupErr != nil {
			return Event{}, fmt.Errorf("%s: %w", name, lookupErr)
		}
		if !exists {
			continue
		}
		switch name {
		case "actual_arrival_at":
			result.ActualArrivalAt = &value
		case "actual_departure_at":
			result.ActualDepartureAt = &value
		case "scheduled_at":
			result.ScheduledAt = &value
		}
	}
	if mapping.Reason != "" {
		if value, exists, lookupErr := optionalString(row, mapping.Reason); lookupErr != nil {
			return Event{}, fmt.Errorf("reason: %w", lookupErr)
		} else if exists {
			result.Reason = value
		}
	}
	return result, nil
}

func normalizeStatus(raw string, mapping map[string]string) string {
	value := strings.ToLower(strings.TrimSpace(raw))
	if mapped, ok := mapping[raw]; ok {
		value = strings.ToLower(strings.TrimSpace(mapped))
	} else if mapped, ok := mapping[strings.ToLower(strings.TrimSpace(raw))]; ok {
		value = strings.ToLower(strings.TrimSpace(mapped))
	}
	switch value {
	case "arrival", "landed":
		return "arrived"
	case "departure", "takeoff", "departed":
		return "departed"
	case "cancel", "canceled", "cancelled":
		return "cancelled"
	case "delay", "delayed":
		return "delayed"
	case "arrived":
		return value
	default:
		return ""
	}
}

func requiredString(row map[string]any, path string) (string, error) {
	value, exists, err := optionalString(row, path)
	if err != nil {
		return "", err
	}
	if !exists || strings.TrimSpace(value) == "" {
		return "", fmt.Errorf("required field %q is missing", path)
	}
	return strings.TrimSpace(value), nil
}

func optionalString(row map[string]any, path string) (string, bool, error) {
	value, err := lookupPath(row, path)
	if err != nil {
		if errors.Is(err, errPathMissing) {
			return "", false, nil
		}
		return "", false, err
	}
	switch typed := value.(type) {
	case nil:
		return "", false, nil
	case string:
		return typed, true, nil
	case json.Number:
		return typed.String(), true, nil
	case float64:
		return strconv.FormatFloat(typed, 'f', -1, 64), true, nil
	case bool:
		return strconv.FormatBool(typed), true, nil
	default:
		return "", false, fmt.Errorf("field %q must be a scalar", path)
	}
}

func requiredTime(row map[string]any, path string, formats []string) (time.Time, error) {
	value, exists, err := optionalTime(row, path, formats)
	if err != nil {
		return time.Time{}, err
	}
	if !exists {
		return time.Time{}, fmt.Errorf("required time field %q is missing", path)
	}
	return value, nil
}

func optionalTime(row map[string]any, path string, formats []string) (time.Time, bool, error) {
	value, exists, err := optionalString(row, path)
	if err != nil || !exists || strings.TrimSpace(value) == "" {
		return time.Time{}, exists, err
	}
	parsers := append([]string{time.RFC3339Nano, time.RFC3339, "2006-01-02 15:04:05.999999", "2006-01-02 15:04:05", "2006-01-02"}, formats...)
	for _, format := range parsers {
		if strings.EqualFold(strings.TrimSpace(format), "rfc3339") {
			format = time.RFC3339
		}
		if parsed, parseErr := time.Parse(format, strings.TrimSpace(value)); parseErr == nil {
			return parsed.UTC(), true, nil
		}
		if parsed, parseErr := time.ParseInLocation(format, strings.TrimSpace(value), time.UTC); parseErr == nil {
			return parsed.UTC(), true, nil
		}
	}
	return time.Time{}, true, fmt.Errorf("time field %q has unsupported value %q", path, value)
}

var errPathMissing = errors.New("path is missing")

func lookupPath(document any, path string) (any, error) {
	value := document
	for _, segment := range strings.Split(strings.Trim(strings.TrimSpace(path), "."), ".") {
		if segment == "" {
			continue
		}
		object, ok := value.(map[string]any)
		if !ok {
			return nil, fmt.Errorf("cannot read segment %q", segment)
		}
		value, ok = object[segment]
		if !ok {
			return nil, fmt.Errorf("%w: %q", errPathMissing, segment)
		}
	}
	return value, nil
}
