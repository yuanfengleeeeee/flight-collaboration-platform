package observability

import (
	"database/sql"
	"fmt"
	"net/http"
	"sort"
	"strconv"
	"strings"
	"sync"
)

// Labels is intentionally a small, low-cardinality label set. Callers should
// use route templates and stable operation/result names, never raw URLs or
// user identifiers.
type Labels map[string]string

type metricKind string

const (
	metricCounter   metricKind = "counter"
	metricGauge     metricKind = "gauge"
	metricHistogram metricKind = "histogram"
)

type metricDefinition struct {
	name    string
	help    string
	kind    metricKind
	buckets []float64
}

type metricLabel struct {
	name  string
	value string
}

type metricSample struct {
	labels  []metricLabel
	counter float64
	gauge   float64
	buckets []uint64
	count   uint64
	sum     float64
}

// Registry is a small Prometheus text-format registry. It keeps the project
// free of a mandatory metrics dependency while still exposing histograms from
// which a scraper can calculate p50/p95/p99.
type Registry struct {
	mu          sync.RWMutex
	definitions map[string]metricDefinition
	samples     map[string]map[string]*metricSample
}

func NewRegistry() *Registry {
	return &Registry{definitions: make(map[string]metricDefinition), samples: make(map[string]map[string]*metricSample)}
}

func NewHTTPRegistry() *Registry {
	registry := NewRegistry()
	RegisterHTTPMetrics(registry)
	RegisterDatabaseMetrics(registry)
	RegisterQueueMetrics(registry)
	RegisterNotificationMetrics(registry)
	RegisterRealtimeMetrics(registry)
	return registry
}

func NewWorkerRegistry() *Registry {
	registry := NewRegistry()
	RegisterWorkerMetrics(registry)
	RegisterDatabaseMetrics(registry)
	RegisterQueueMetrics(registry)
	return registry
}

func RegisterHTTPMetrics(registry *Registry) {
	if registry == nil {
		return
	}
	registry.RegisterCounter("flight_http_requests_total", "Total HTTP requests handled by the service.")
	registry.RegisterHistogram("flight_http_request_duration_seconds", "HTTP request duration in seconds.", defaultDurationBuckets())
}

func RegisterDatabaseMetrics(registry *Registry) {
	if registry == nil {
		return
	}
	registry.RegisterGauge("flight_db_open_connections", "Current open database connections.")
	registry.RegisterGauge("flight_db_in_use_connections", "Current database connections in use.")
	registry.RegisterGauge("flight_db_idle_connections", "Current idle database connections.")
	registry.RegisterGauge("flight_db_wait_count", "Database connection wait count reported by database/sql.")
	registry.RegisterGauge("flight_db_wait_duration_seconds", "Database connection wait duration in seconds reported by database/sql.")
}

func RegisterQueueMetrics(registry *Registry) {
	if registry == nil {
		return
	}
	registry.RegisterGauge("flight_sync_queue_pending_items", "Current pending items in a durable synchronization queue.")
	registry.RegisterGauge("flight_sync_queue_failed_items", "Current failed items in a durable synchronization queue.")
	registry.RegisterGauge("flight_sync_queue_oldest_age_seconds", "Age in seconds of the oldest pending synchronization item.")
	registry.RegisterGauge("flight_sync_projection_lag_seconds", "Age in seconds between the latest applied event and now.")
}

func RegisterWorkerMetrics(registry *Registry) {
	if registry == nil {
		return
	}
	registry.RegisterCounter("flight_worker_outbox_claimed_total", "Core Outbox records claimed by the Worker.")
	registry.RegisterCounter("flight_worker_outbox_delivery_total", "Core Outbox delivery results.")
	registry.RegisterHistogram("flight_worker_outbox_delivery_duration_seconds", "Core Outbox delivery duration in seconds.", defaultDurationBuckets())
	registry.RegisterCounter("flight_worker_command_batches_total", "Edge Command pull batches by result.")
	registry.RegisterCounter("flight_worker_commands_claimed_total", "Edge Commands claimed by the Worker.")
	registry.RegisterCounter("flight_worker_command_processing_total", "Core Command processing results.")
	registry.RegisterHistogram("flight_worker_command_processing_duration_seconds", "Core Command processing duration in seconds.", defaultDurationBuckets())
	registry.RegisterCounter("flight_worker_command_ack_total", "Edge Command acknowledgement results.")
	registry.RegisterHistogram("flight_worker_command_ack_duration_seconds", "Edge Command acknowledgement duration in seconds.", defaultDurationBuckets())
	registry.RegisterHistogram("flight_worker_cycle_duration_seconds", "Worker operation cycle duration in seconds.", defaultDurationBuckets())
	registry.RegisterCounter("flight_worker_errors_total", "Worker operation errors.")
}

func RegisterNotificationMetrics(registry *Registry) {
	if registry == nil {
		return
	}
	registry.RegisterCounter("flight_notification_delivery_total", "Best-effort Edge notification delivery results.")
}

func RegisterRealtimeMetrics(registry *Registry) {
	if registry == nil {
		return
	}
	registry.RegisterGauge("flight_websocket_connections", "Current live Edge employee WebSocket connections.")
	registry.RegisterCounter("flight_websocket_connections_total", "Edge employee WebSocket connection results.")
	registry.RegisterCounter("flight_websocket_disconnects_total", "Edge employee WebSocket disconnect results.")
	registry.RegisterCounter("flight_websocket_heartbeat_total", "Edge employee WebSocket heartbeat messages.")
	registry.RegisterHistogram("flight_websocket_notification_delivery_seconds", "Time from task notification issuance to WebSocket delivery.", defaultDurationBuckets())
}

func (r *Registry) RegisterCounter(name, help string) {
	r.register(metricDefinition{name: name, help: help, kind: metricCounter})
}

func (r *Registry) RegisterGauge(name, help string) {
	r.register(metricDefinition{name: name, help: help, kind: metricGauge})
}

func (r *Registry) RegisterHistogram(name, help string, buckets []float64) {
	copyBuckets := append([]float64(nil), buckets...)
	sort.Float64s(copyBuckets)
	r.register(metricDefinition{name: name, help: help, kind: metricHistogram, buckets: copyBuckets})
}

func (r *Registry) register(definition metricDefinition) {
	if r == nil || definition.name == "" {
		return
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	if _, exists := r.definitions[definition.name]; !exists {
		r.definitions[definition.name] = definition
	}
}

func (r *Registry) Inc(name string, labels Labels) { r.Add(name, labels, 1) }

func (r *Registry) Add(name string, labels Labels, value float64) {
	if r == nil || value == 0 {
		return
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	definition, ok := r.definitions[name]
	if !ok || definition.kind != metricCounter {
		return
	}
	sample := r.sampleLocked(definition, labels)
	sample.counter += value
}

func (r *Registry) SetGauge(name string, labels Labels, value float64) {
	if r == nil {
		return
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	definition, ok := r.definitions[name]
	if !ok || definition.kind != metricGauge {
		return
	}
	sample := r.sampleLocked(definition, labels)
	sample.gauge = value
}

func (r *Registry) Observe(name string, labels Labels, value float64) {
	if r == nil {
		return
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	definition, ok := r.definitions[name]
	if !ok || definition.kind != metricHistogram {
		return
	}
	sample := r.sampleLocked(definition, labels)
	for index, bucket := range definition.buckets {
		if value <= bucket {
			sample.buckets[index]++
		}
	}
	sample.count++
	sample.sum += value
}

func (r *Registry) sampleLocked(definition metricDefinition, labels Labels) *metricSample {
	key, normalized := normalizeLabels(labels)
	family := r.samples[definition.name]
	if family == nil {
		family = make(map[string]*metricSample)
		r.samples[definition.name] = family
	}
	if sample, ok := family[key]; ok {
		return sample
	}
	sample := &metricSample{labels: normalized}
	if definition.kind == metricHistogram {
		sample.buckets = make([]uint64, len(definition.buckets))
	}
	family[key] = sample
	return sample
}

func (r *Registry) Handler() http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "text/plain; version=0.0.4; charset=utf-8")
		_, _ = w.Write([]byte(r.Render()))
	})
}

func (r *Registry) Render() string {
	if r == nil {
		return ""
	}
	r.mu.RLock()
	defer r.mu.RUnlock()

	names := make([]string, 0, len(r.definitions))
	for name := range r.definitions {
		names = append(names, name)
	}
	sort.Strings(names)
	var output strings.Builder
	for _, name := range names {
		definition := r.definitions[name]
		fmt.Fprintf(&output, "# HELP %s %s\n# TYPE %s %s\n", definition.name, definition.help, definition.name, definition.kind)
		keys := make([]string, 0, len(r.samples[name]))
		for key := range r.samples[name] {
			keys = append(keys, key)
		}
		sort.Strings(keys)
		for _, key := range keys {
			sample := r.samples[name][key]
			switch definition.kind {
			case metricCounter:
				fmt.Fprintf(&output, "%s%s %s\n", name, renderLabels(sample.labels), formatFloat(sample.counter))
			case metricGauge:
				fmt.Fprintf(&output, "%s%s %s\n", name, renderLabels(sample.labels), formatFloat(sample.gauge))
			case metricHistogram:
				for index, bucket := range definition.buckets {
					labels := append([]metricLabel(nil), sample.labels...)
					labels = append(labels, metricLabel{name: "le", value: formatFloat(bucket)})
					sort.Slice(labels, func(i, j int) bool { return labels[i].name < labels[j].name })
					fmt.Fprintf(&output, "%s_bucket%s %d\n", name, renderLabels(labels), sample.buckets[index])
				}
				labels := append([]metricLabel(nil), sample.labels...)
				labels = append(labels, metricLabel{name: "le", value: "+Inf"})
				sort.Slice(labels, func(i, j int) bool { return labels[i].name < labels[j].name })
				fmt.Fprintf(&output, "%s_bucket%s %d\n", name, renderLabels(labels), sample.count)
				fmt.Fprintf(&output, "%s_sum%s %s\n", name, renderLabels(sample.labels), formatFloat(sample.sum))
				fmt.Fprintf(&output, "%s_count%s %d\n", name, renderLabels(sample.labels), sample.count)
			}
		}
	}
	return output.String()
}

func normalizeLabels(labels Labels) (string, []metricLabel) {
	names := make([]string, 0, len(labels))
	for name := range labels {
		if name != "" {
			names = append(names, name)
		}
	}
	sort.Strings(names)
	result := make([]metricLabel, 0, len(names))
	var key strings.Builder
	for _, name := range names {
		value := labels[name]
		result = append(result, metricLabel{name: name, value: value})
		key.WriteString(name)
		key.WriteByte('=')
		key.WriteString(value)
		key.WriteByte('\x00')
	}
	return key.String(), result
}

func renderLabels(labels []metricLabel) string {
	if len(labels) == 0 {
		return ""
	}
	var output strings.Builder
	output.WriteByte('{')
	for index, label := range labels {
		if index > 0 {
			output.WriteByte(',')
		}
		fmt.Fprintf(&output, "%s=\"%s\"", label.name, escapeLabel(label.value))
	}
	output.WriteByte('}')
	return output.String()
}

func escapeLabel(value string) string {
	value = strings.ReplaceAll(value, `\`, `\\`)
	value = strings.ReplaceAll(value, `"`, `\"`)
	return strings.ReplaceAll(value, "\n", `\n`)
}

func formatFloat(value float64) string {
	return strconv.FormatFloat(value, 'g', -1, 64)
}

func defaultDurationBuckets() []float64 {
	return []float64{0.001, 0.005, 0.01, 0.025, 0.05, 0.1, 0.25, 0.5, 1, 2.5, 5, 10}
}

// RecordDBStats snapshots database/sql pool state. It deliberately exposes
// pool pressure rather than SQL text, which avoids leaking query contents and
// keeps labels stable enough for a long-running service.
func RecordDBStats(registry *Registry, component string, db *sql.DB) {
	if registry == nil || db == nil {
		return
	}
	stats := db.Stats()
	labels := Labels{"component": component}
	registry.SetGauge("flight_db_open_connections", labels, float64(stats.OpenConnections))
	registry.SetGauge("flight_db_in_use_connections", labels, float64(stats.InUse))
	registry.SetGauge("flight_db_idle_connections", labels, float64(stats.Idle))
	registry.SetGauge("flight_db_wait_count", labels, float64(stats.WaitCount))
	registry.SetGauge("flight_db_wait_duration_seconds", labels, stats.WaitDuration.Seconds())
}
