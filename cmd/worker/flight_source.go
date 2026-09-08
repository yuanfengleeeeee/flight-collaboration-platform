package main

import (
	"fmt"
	"net/http"
	"time"

	flightintegration "github.com/yuanfengleeeeee/flight-collaboration-platform/internal/integration/flight"
	"github.com/yuanfengleeeeee/flight-collaboration-platform/internal/platform/config"
	platformsecurity "github.com/yuanfengleeeeee/flight-collaboration-platform/internal/platform/security"
)

func newConfiguredFlightSourceProvider(source config.FlightSourceConfig) (flightintegration.Provider, error) {
	token, username, password, err := source.ResolveAuth()
	if err != nil {
		return nil, err
	}
	tlsConfig, err := platformsecurity.NewClientTLSConfig(source.TLS)
	if err != nil {
		return nil, fmt.Errorf("configure flight source TLS: %w", err)
	}
	client := &http.Client{Timeout: time.Duration(source.TimeoutSeconds) * time.Second, Transport: &http.Transport{TLSClientConfig: tlsConfig, MaxIdleConns: 10, MaxIdleConnsPerHost: 4, IdleConnTimeout: 90 * time.Second}}
	provider, err := flightintegration.NewHTTPJSONProvider(flightintegration.HTTPJSONProviderConfig{
		Name:          source.Provider,
		BaseURL:       source.BaseURL,
		SchedulesPath: source.SchedulesPath,
		EventsPath:    source.EventsPath,
		FromQuery:     source.FromQuery,
		ToQuery:       source.ToQuery,
		Timeout:       time.Duration(source.TimeoutSeconds) * time.Second,
		UserAgent:     source.UserAgent,
		Headers:       source.Headers,
		Auth: flightintegration.HTTPJSONAuthConfig{
			Type:         source.Auth.Type,
			Token:        token,
			APIKeyHeader: source.Auth.APIKeyHeader,
			Username:     username,
			Password:     password,
		},
		ScheduleMapping: flightintegration.ScheduleMapping{
			RecordsPath:      source.ScheduleMapping.RecordsPath,
			ExternalFlightID: source.ScheduleMapping.ExternalFlightID,
			FlightDisplayNo:  source.ScheduleMapping.FlightDisplayNo,
			OperatingDate:    source.ScheduleMapping.OperatingDate,
			ScheduledAt:      source.ScheduleMapping.ScheduledAt,
		},
		EventMapping: flightintegration.EventMapping{
			RecordsPath:       source.EventMapping.RecordsPath,
			ExternalEventID:   source.EventMapping.ExternalEventID,
			ExternalFlightID:  source.EventMapping.ExternalFlightID,
			Status:            source.EventMapping.Status,
			OccurredAt:        source.EventMapping.OccurredAt,
			ActualArrivalAt:   source.EventMapping.ActualArrivalAt,
			ActualDepartureAt: source.EventMapping.ActualDepartureAt,
			ScheduledAt:       source.EventMapping.ScheduledAt,
			Reason:            source.EventMapping.Reason,
			StatusMap:         source.EventMapping.StatusMap,
		},
		TimeFormats: source.TimeFormats,
	}, client)
	if err != nil {
		return nil, err
	}
	return provider, nil
}

func newFlightSourceNotifier(source config.FlightSourceConfig) (flightintegration.Notifier, error) {
	if !source.Alert.Enabled {
		return nil, nil
	}
	webhookURL, err := source.Alert.ResolveWebhookURL()
	if err != nil {
		return nil, err
	}
	timeout := time.Duration(source.Alert.TimeoutSeconds) * time.Second
	if timeout <= 0 {
		timeout = 5 * time.Second
	}
	return flightintegration.NewWebhookNotifier(webhookURL, &http.Client{Timeout: timeout})
}
