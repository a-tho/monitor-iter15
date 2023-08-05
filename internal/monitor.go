package monitor

import (
	"context"
	"io"
)

type Gauge float64

type Counter int64

type Metrics struct {
	ID    string   `json:"id"`              // metric name
	MType string   `json:"type"`            // parameter, taking a value of gauge or counter
	Delta *int64   `json:"delta,omitempty"` // metric value in case of a counter
	Value *float64 `json:"value,omitempty"` // metric value in case of a gauge
}

// A MetricRepo is used for a single metric type (e.g. gauge or counter) and
// stores a value for each metric name.
type MetricRepo interface {
	SetGauge(ctx context.Context, k string, v Gauge) (MetricRepo, error)
	SetGaugeBatch(ctx context.Context, batch []*Metrics) (MetricRepo, error)
	GetGauge(ctx context.Context, k string) (v Gauge, ok bool)
	StringGauge(ctx context.Context) (string, error)
	WriteAllGauge(ctx context.Context, wr io.Writer) error

	AddCounter(ctx context.Context, k string, v Counter) (MetricRepo, error)
	AddCounterBatch(ctx context.Context, batch []*Metrics) (MetricRepo, error)
	GetCounter(ctx context.Context, k string) (v Counter, ok bool)
	StringCounter(ctx context.Context) (string, error)
	WriteAllCounter(ctx context.Context, wr io.Writer) error

	PingContext(ctx context.Context) error
	Close() error
}

// An Observer is used to collect and transmit metrics.
type Observer interface {
	Observe(ctx context.Context) error
}
