package flightsync

import (
	"context"
	"fmt"
	"strings"
	"time"

	flightintegration "github.com/yuanfengleeeeee/flight-collaboration-platform/internal/integration/flight"
	"go.uber.org/zap"
)

type Poller struct {
	service                       *Service
	provider                      flightintegration.Provider
	interval                      time.Duration
	lookback                      time.Duration
	lookahead                     time.Duration
	notifier                      flightintegration.Notifier
	log                           *zap.Logger
	now                           func() time.Time
	reconcile                     bool
	alertOnSyncFailure            bool
	alertOnReconciliationMismatch bool
}

func NewPoller(service *Service, provider flightintegration.Provider, interval, lookback, lookahead time.Duration) *Poller {
	if interval <= 0 {
		interval = time.Minute
	}
	if lookback < 0 {
		lookback = time.Hour
	}
	if lookahead <= 0 {
		lookahead = 24 * time.Hour
	}
	return &Poller{service: service, provider: provider, interval: interval, lookback: lookback, lookahead: lookahead, log: zap.NewNop(), now: func() time.Time { return time.Now().UTC() }, reconcile: true, alertOnSyncFailure: true, alertOnReconciliationMismatch: true}
}

func (p *Poller) SetNotifier(notifier flightintegration.Notifier) *Poller {
	if p != nil {
		p.notifier = notifier
	}
	return p
}

func (p *Poller) SetLogger(log *zap.Logger) *Poller {
	if p != nil && log != nil {
		p.log = log
	}
	return p
}

func (p *Poller) SetClock(now func() time.Time) *Poller {
	if p != nil && now != nil {
		p.now = now
	}
	return p
}

func (p *Poller) SetReconciliationEnabled(enabled bool) *Poller {
	if p != nil {
		p.reconcile = enabled
	}
	return p
}

func (p *Poller) SetAlertPolicy(onSyncFailure, onReconciliationMismatch bool) *Poller {
	if p != nil {
		p.alertOnSyncFailure = onSyncFailure
		p.alertOnReconciliationMismatch = onReconciliationMismatch
	}
	return p
}

func (p *Poller) Run(ctx context.Context) {
	if p == nil {
		return
	}
	p.runAndLog(ctx)
	ticker := time.NewTicker(p.interval)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			p.runAndLog(ctx)
		}
	}
}

func (p *Poller) RunOnce(ctx context.Context) (SyncWindowResult, ReconciliationResult, error) {
	if p == nil || p.service == nil || p.provider == nil {
		return SyncWindowResult{}, ReconciliationResult{}, fmt.Errorf("flight source poller is not configured")
	}
	now := p.now().UTC()
	from := now.Add(-p.lookback)
	to := now.Add(p.lookahead)
	result, err := p.service.SyncWindowWithRecords(ctx, p.provider, from, to)
	if err != nil {
		if p.alertOnSyncFailure {
			p.notify(ctx, flightintegration.Alert{Type: flightintegration.AlertSourceSyncFailure, Severity: "critical", Provider: p.provider.Name(), Message: "flight source polling failed", At: now, Details: map[string]any{"window_from": from, "window_to": to, "error": trimAlertError(err)}})
		}
		return SyncWindowResult{}, ReconciliationResult{}, err
	}
	if !p.reconcile {
		return result, ReconciliationResult{}, nil
	}
	reconciliation, err := p.service.ReconcileSchedules(ctx, p.provider.Name(), from, to, result.Schedules, now)
	if err != nil {
		if p.alertOnReconciliationMismatch {
			p.notify(ctx, flightintegration.Alert{Type: flightintegration.AlertSourceReconciliationFailed, Severity: "warning", Provider: p.provider.Name(), Message: "flight source reconciliation failed", At: now, Details: map[string]any{"window_from": from, "window_to": to, "error": trimAlertError(err)}})
		}
		return result, ReconciliationResult{}, err
	}
	if reconciliation.Status == "mismatch" {
		if p.alertOnReconciliationMismatch {
			p.notify(ctx, flightintegration.Alert{Type: flightintegration.AlertSourceMismatch, Severity: "warning", Provider: p.provider.Name(), Message: "flight source reconciliation mismatch", At: now, Details: map[string]any{"window_from": from, "window_to": to, "upstream_count": reconciliation.UpstreamCount, "core_count": reconciliation.CoreCount, "upstream_missing_count": reconciliation.UpstreamMissingCount, "core_missing_count": reconciliation.CoreMissingCount, "pending_apply_count": reconciliation.PendingApplyCount, "upstream_missing_ids": reconciliation.UpstreamMissingIDs, "core_missing_ids": reconciliation.CoreMissingIDs}})
		}
	}
	return result, reconciliation, nil
}

func (p *Poller) runAndLog(ctx context.Context) {
	_, reconciliation, err := p.RunOnce(ctx)
	if err != nil {
		p.log.Warn("flight source poll failed", zap.String("provider", p.providerName()), zap.Error(err))
		return
	}
	p.log.Info("flight source poll completed", zap.String("provider", p.providerName()), zap.String("reconciliation_status", reconciliation.Status))
}

func (p *Poller) notify(ctx context.Context, alert flightintegration.Alert) {
	if p == nil || p.notifier == nil {
		return
	}
	if err := p.notifier.Notify(ctx, alert); err != nil {
		p.log.Warn("flight source alert delivery failed", zap.String("alert_type", alert.Type), zap.String("provider", alert.Provider), zap.Error(err))
	}
}

func (p *Poller) providerName() string {
	if p == nil || p.provider == nil {
		return ""
	}
	return strings.TrimSpace(p.provider.Name())
}

func trimAlertError(err error) string {
	value := strings.TrimSpace(err.Error())
	if len(value) > 1024 {
		return value[:1024]
	}
	return value
}
