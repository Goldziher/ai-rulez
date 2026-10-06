package telemetry

import (
	"encoding/json"
	"hash/fnv"
	"maps"
	"slices"
	"sort"
	"strconv"
	"strings"
	"time"
)

// This file is a small OTLP/HTTP JSON encoder (lowerCamelCase keys, 64-bit
// integers as decimal strings, enums as integers), written by hand instead of
// pulling in the OpenTelemetry SDK: see docs/telemetry.md for the dependency
// decision.

// Metric names.
const (
	MetricLoads     = "ai_rulez.item.loads"
	MetricOutcomes  = "ai_rulez.item.outcomes"
	MetricAgentTime = "ai_rulez.agent.duration"
	scopeName       = "ai-rulez.telemetry"

	severityInfo        = 9
	temporalityDelta    = 1
	logEventNamePrefix  = "ai_rulez.item."
	resourceSchemaKey   = "ai_rulez.schema_version"
	resourceServiceName = "service.name"
	resourceServiceVer  = "service.version"
)

// agentBuckets are the explicit histogram bounds for agent duration, in ms.
var agentBuckets = []float64{100, 500, 1000, 5000, 15000, 60000, 300000}

type otlpValue struct {
	String *string `json:"stringValue,omitempty"`
	Bool   *bool   `json:"boolValue,omitempty"`
	Int    *string `json:"intValue,omitempty"`
}

type otlpAttr struct {
	Key   string    `json:"key"`
	Value otlpValue `json:"value"`
}

func strAttr(key, value string) otlpAttr { return otlpAttr{Key: key, Value: otlpValue{String: &value}} }
func boolAttr(key string, value bool) otlpAttr {
	return otlpAttr{Key: key, Value: otlpValue{Bool: &value}}
}
func intAttr(key string, value int64) otlpAttr {
	text := strconv.FormatInt(value, 10)
	return otlpAttr{Key: key, Value: otlpValue{Int: &text}}
}

type otlpResource struct {
	Attributes []otlpAttr `json:"attributes"`
}

type otlpScope struct {
	Name    string `json:"name"`
	Version string `json:"version"`
}

// Encoder maps events to OTLP JSON bodies.
type Encoder struct {
	ServiceName    string
	ServiceVersion string
	// Resource is the validated [telemetry.resource] table, emitted sorted by key
	// after the fixed attributes.
	Resource map[string]string
	// IncludePaths and IncludeSession open the gated attributes.
	IncludePaths   bool
	IncludeSession bool
}

func (en *Encoder) resource() otlpResource {
	name := en.ServiceName
	if name == "" {
		name = DefaultServiceName
	}
	attrs := []otlpAttr{strAttr(resourceServiceName, name)}
	if en.ServiceVersion != "" {
		attrs = append(attrs, strAttr(resourceServiceVer, en.ServiceVersion))
	}
	attrs = append(attrs, strAttr(resourceSchemaKey, strconv.Itoa(SchemaVersion)))
	for _, key := range slices.Sorted(maps.Keys(en.Resource)) {
		attrs = append(attrs, strAttr(key, en.Resource[key]))
	}
	return otlpResource{Attributes: attrs}
}

func (en *Encoder) open(gate string) bool {
	switch gate {
	case "":
		return true
	case GatePaths:
		return en.IncludePaths
	case GateSession:
		return en.IncludeSession
	}
	return false
}

// fieldValue returns the attribute for an allowlist row, false when the event
// has no value for it.
func fieldValue(a Attr, e *Event) (otlpAttr, bool) {
	switch a.Field {
	case fieldServed:
		return boolAttr(a.Name, e.Served), true
	case fieldDuration:
		if e.DurationMS > 0 {
			return intAttr(a.Name, e.DurationMS), true
		}
		return otlpAttr{}, false
	}
	if value := textField(a.Field, e); value != "" {
		return strAttr(a.Name, value), true
	}
	return otlpAttr{}, false
}

// textField returns the string form of a text field of the event.
func textField(field string, e *Event) string {
	switch field {
	case fieldKind:
		return e.Kind
	case fieldID:
		return e.ID
	case fieldDigest:
		return e.Digest
	case fieldPath:
		return e.Path
	case fieldSource:
		return e.Source
	case fieldHarness:
		return e.Harness
	case fieldRole:
		return e.Role
	case fieldSession:
		return e.Session
	case fieldOutcome:
		return e.Outcome
	case fieldLoadReason:
		return e.LoadReason
	case fieldMemoryType:
		return e.MemoryType
	case fieldEventID:
		return e.EventID
	}
	return ""
}

type otlpLogRecord struct {
	TimeUnixNano         string     `json:"timeUnixNano"`
	ObservedTimeUnixNano string     `json:"observedTimeUnixNano"`
	SeverityNumber       int        `json:"severityNumber"`
	SeverityText         string     `json:"severityText"`
	Body                 otlpValue  `json:"body"`
	Attributes           []otlpAttr `json:"attributes"`
}

type scopeLogs struct {
	Scope      otlpScope       `json:"scope"`
	LogRecords []otlpLogRecord `json:"logRecords"`
}

type resourceLogs struct {
	Resource  otlpResource `json:"resource"`
	ScopeLogs []scopeLogs  `json:"scopeLogs"`
}

type logsRequest struct {
	ResourceLogs []resourceLogs `json:"resourceLogs"`
}

func unixNano(ts string, fallback time.Time) string {
	t, err := time.Parse(time.RFC3339, ts)
	if err != nil {
		t = fallback
	}
	return nanos(t)
}

// nanos renders t as OTLP nanoseconds since the epoch. A zero time or one outside
// the int64 nanosecond range (years 1678 to 2261) has no valid value: it becomes
// "0", which OTLP reads as unknown, instead of an overflowed number.
func nanos(t time.Time) string {
	if !nanosecondsRepresentable(t) {
		return "0"
	}
	return strconv.FormatInt(t.UnixNano(), 10)
}

func nanosecondsRepresentable(t time.Time) bool {
	return !t.IsZero() && t.Year() > 1677 && t.Year() < 2262
}

// EncodeLogs renders one LogRecord per event. observed is the flush time.
func (en *Encoder) EncodeLogs(events []Event, observed time.Time) ([]byte, error) {
	records := make([]otlpLogRecord, 0, len(events))
	for i := range events {
		e := &events[i]
		attrs := []otlpAttr{strAttr("event.name", logEventNamePrefix+e.Outcome)}
		for _, a := range Allowlist {
			if !en.open(a.Gate) {
				continue
			}
			if attr, ok := fieldValue(a, e); ok {
				attrs = append(attrs, attr)
			}
		}
		body := "item " + e.Outcome
		records = append(records, otlpLogRecord{
			TimeUnixNano:         unixNano(e.Time, observed),
			ObservedTimeUnixNano: nanos(observed),
			SeverityNumber:       severityInfo, SeverityText: "INFO",
			Body: otlpValue{String: &body}, Attributes: attrs,
		})
	}
	req := logsRequest{ResourceLogs: []resourceLogs{{
		Resource: en.resource(),
		ScopeLogs: []scopeLogs{{
			Scope:      otlpScope{Name: scopeName, Version: strconv.Itoa(SchemaVersion)},
			LogRecords: records,
		}},
	}}}
	return json.Marshal(req)
}

type numberPoint struct {
	Attributes        []otlpAttr `json:"attributes"`
	StartTimeUnixNano string     `json:"startTimeUnixNano"`
	TimeUnixNano      string     `json:"timeUnixNano"`
	AsInt             string     `json:"asInt"`
}

type histogramPoint struct {
	Attributes        []otlpAttr `json:"attributes"`
	StartTimeUnixNano string     `json:"startTimeUnixNano"`
	TimeUnixNano      string     `json:"timeUnixNano"`
	Count             string     `json:"count"`
	Sum               float64    `json:"sum"`
	BucketCounts      []string   `json:"bucketCounts"`
	ExplicitBounds    []float64  `json:"explicitBounds"`
	Min               float64    `json:"min"`
	Max               float64    `json:"max"`
}

type otlpSum struct {
	AggregationTemporality int           `json:"aggregationTemporality"`
	IsMonotonic            bool          `json:"isMonotonic"`
	DataPoints             []numberPoint `json:"dataPoints"`
}

type otlpHistogram struct {
	AggregationTemporality int              `json:"aggregationTemporality"`
	DataPoints             []histogramPoint `json:"dataPoints"`
}

type otlpMetric struct {
	Name        string         `json:"name"`
	Description string         `json:"description"`
	Unit        string         `json:"unit"`
	Sum         *otlpSum       `json:"sum,omitempty"`
	Histogram   *otlpHistogram `json:"histogram,omitempty"`
}

type scopeMetrics struct {
	Scope   otlpScope    `json:"scope"`
	Metrics []otlpMetric `json:"metrics"`
}

type resourceMetrics struct {
	Resource     otlpResource   `json:"resource"`
	ScopeMetrics []scopeMetrics `json:"scopeMetrics"`
}

type metricsRequest struct {
	ResourceMetrics []resourceMetrics `json:"resourceMetrics"`
}

// metricLabels builds the label set of an event for the counters. Only
// allowlist rows with a Metric name and no gate are labels; session and path
// never are. The outcome label is added for the outcomes counter only, since
// ai_rulez.item.loads counts outcome=loaded by definition.
func metricLabels(e *Event, withOutcome bool) (attrs []otlpAttr, key string) {
	for _, a := range Allowlist {
		if a.Metric == "" || a.Gate != "" || (a.Field == fieldOutcome && !withOutcome) {
			continue
		}
		attr, ok := fieldValue(a, e)
		if !ok {
			continue
		}
		attr.Key = a.Metric
		if a.Field == fieldDigest {
			short := digestShort(e.Digest)
			attr.Value = otlpValue{String: &short}
		}
		attrs = append(attrs, attr)
	}
	sort.Slice(attrs, func(i, j int) bool { return attrs[i].Key < attrs[j].Key })
	parts := make([]string, 0, len(attrs))
	for _, attr := range attrs {
		parts = append(parts, attr.Key+"="+labelText(attr.Value))
	}
	return attrs, strings.Join(parts, "\x00")
}

func labelText(v otlpValue) string {
	switch {
	case v.String != nil:
		return *v.String
	case v.Bool != nil:
		return strconv.FormatBool(*v.Bool)
	case v.Int != nil:
		return *v.Int
	}
	return ""
}

type bucket struct {
	attrs []otlpAttr
	count int64
}

type hist struct {
	attrs    []otlpAttr
	counts   []int64
	sum      float64
	min, max float64
	count    int64
}

// aggregator folds a batch of events into the three delta metrics.
type aggregator struct {
	loads, outcomes map[string]*bucket
	durations       map[string]*hist
}

func newAggregator() *aggregator {
	return &aggregator{loads: map[string]*bucket{}, outcomes: map[string]*bucket{}, durations: map[string]*hist{}}
}

func (ag *aggregator) add(e *Event) {
	target, withOutcome := ag.outcomes, true
	if e.Outcome == OutcomeLoaded {
		target, withOutcome = ag.loads, false
	}
	attrs, key := metricLabels(e, withOutcome)
	if target[key] == nil {
		target[key] = &bucket{attrs: attrs}
	}
	target[key].count++
	if e.Kind == KindAgent && e.DurationMS > 0 {
		ag.addDuration(e)
	}
}

func (ag *aggregator) addDuration(e *Event) {
	attrs := []otlpAttr{strAttr(fieldKind, e.Kind), strAttr(fieldID, e.ID)}
	if e.Harness != "" {
		attrs = append(attrs, strAttr(fieldHarness, e.Harness))
	}
	key := e.ID + "\x00" + e.Harness
	v := float64(e.DurationMS)
	h := ag.durations[key]
	if h == nil {
		h = &hist{attrs: attrs, counts: make([]int64, len(agentBuckets)+1), min: v, max: v}
		ag.durations[key] = h
	}
	h.counts[sort.SearchFloat64s(agentBuckets, v)]++
	h.sum += v
	h.count++
	h.min, h.max = min(h.min, v), max(h.max, v)
}

func sumMetric(name, desc string, buckets map[string]*bucket, startNano, nowNano string) (otlpMetric, bool) {
	if len(buckets) == 0 {
		return otlpMetric{}, false
	}
	keys := sortedKeys(buckets)
	points := make([]numberPoint, 0, len(keys))
	for _, k := range keys {
		points = append(points, numberPoint{Attributes: buckets[k].attrs, StartTimeUnixNano: startNano, TimeUnixNano: nowNano, AsInt: strconv.FormatInt(buckets[k].count, 10)})
	}
	return otlpMetric{Name: name, Description: desc, Unit: "{load}", Sum: &otlpSum{AggregationTemporality: temporalityDelta, IsMonotonic: true, DataPoints: points}}, true
}

func (ag *aggregator) histogramMetric(startNano, nowNano string) (otlpMetric, bool) {
	if len(ag.durations) == 0 {
		return otlpMetric{}, false
	}
	keys := sortedKeys(ag.durations)
	points := make([]histogramPoint, 0, len(keys))
	for _, k := range keys {
		h := ag.durations[k]
		counts := make([]string, len(h.counts))
		for i, c := range h.counts {
			counts[i] = strconv.FormatInt(c, 10)
		}
		points = append(points, histogramPoint{Attributes: h.attrs, StartTimeUnixNano: startNano, TimeUnixNano: nowNano, Count: strconv.FormatInt(h.count, 10), Sum: h.sum, BucketCounts: counts, ExplicitBounds: agentBuckets, Min: h.min, Max: h.max})
	}
	return otlpMetric{Name: MetricAgentTime, Description: "Subagent run time", Unit: "ms", Histogram: &otlpHistogram{AggregationTemporality: temporalityDelta, DataPoints: points}}, true
}

// EncodeMetrics aggregates a batch into delta metrics: ai_rulez.item.loads
// (outcome loaded), ai_rulez.item.outcomes (used and abandoned) and the
// ai_rulez.agent.duration histogram. The start time is the first event time in
// the batch, now the flush time. It returns nil when the batch yields no metric.
func (en *Encoder) EncodeMetrics(events []Event, now time.Time) ([]byte, error) {
	start := now
	ag := newAggregator()
	for i := range events {
		if t, err := time.Parse(time.RFC3339, events[i].Time); err == nil && t.Before(start) {
			start = t
		}
		ag.add(&events[i])
	}
	startNano, nowNano := nanos(start), nanos(now)

	var metrics []otlpMetric
	if m, ok := sumMetric(MetricLoads, "Items (skills, rules, agents, context files) loaded by a harness", ag.loads, startNano, nowNano); ok {
		metrics = append(metrics, m)
	}
	if m, ok := sumMetric(MetricOutcomes, "Items used or abandoned after loading", ag.outcomes, startNano, nowNano); ok {
		metrics = append(metrics, m)
	}
	if m, ok := ag.histogramMetric(startNano, nowNano); ok {
		metrics = append(metrics, m)
	}
	if len(metrics) == 0 {
		return nil, nil
	}
	req := metricsRequest{ResourceMetrics: []resourceMetrics{{
		Resource: en.resource(),
		ScopeMetrics: []scopeMetrics{{
			Scope:   otlpScope{Name: scopeName, Version: strconv.Itoa(SchemaVersion)},
			Metrics: metrics,
		}},
	}}}
	return json.Marshal(req)
}

func sortedKeys[V any](m map[string]V) []string {
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	return keys
}

// Sampled reports whether an event is inside the export sample. The decision is
// per salted session, so one session is wholly in or out and a dashboard of
// "rules per session" is not skewed; an event without a session is decided by its
// event id.
func Sampled(sample float64, e *Event) bool {
	if sample >= 1 {
		return true
	}
	if sample <= 0 {
		return false
	}
	key := e.Session
	if key == "" {
		key = e.EventID
	}
	h := fnv.New32a()
	_, _ = h.Write([]byte(key)) //nolint:errcheck // hash.Hash.Write never fails
	return float64(h.Sum32()%10000) < sample*10000
}
