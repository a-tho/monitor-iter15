package storage

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"

	monitor "github.com/a-tho/monitor/internal"
)

func TestStorageSetGauge(t *testing.T) {
	type args struct {
		k string
		v monitor.Gauge
	}

	tests := []struct {
		name string
		args args
		want string
	}{
		{
			name: "set metric",
			args: args{k: "Apple", v: monitor.Gauge(3)},
			want: `{"Apple": 3.0}`,
		},
		{
			name: "reset metric",
			args: args{k: "Apple", v: monitor.Gauge(2)},
			want: `{"Apple": 2.0}`,
		},
		{
			name: "set another metric",
			args: args{k: "Cherry", v: monitor.Gauge(79)},
			want: `{"Apple": 2.0, "Cherry": 79.0}`,
		},
	}

	s, err := New(context.Background(), "postgres://postgres:123456@localhost:5432/database", "", 5, false)
	if assert.NoError(t, err) {
		for _, tt := range tests {
			t.Run(tt.name, func(t *testing.T) {
				s.SetGauge(context.TODO(), tt.args.k, tt.args.v)
				gaugeJSON, err := s.StringGauge(context.TODO())
				assert.NoError(t, err)
				assert.JSONEq(t, tt.want, gaugeJSON)
			})
		}
	}
}

func TestStorageAddCounter(t *testing.T) {
	type args struct {
		k string
		v monitor.Counter
	}

	tests := []struct {
		name string
		args args
		want string
	}{
		{
			name: "set metric",
			args: args{k: "Mississippi", v: monitor.Counter(3)},
			want: `{"Mississippi": 3}`,
		},
		{
			name: "reset metric",
			args: args{k: "Mississippi", v: monitor.Counter(2)},
			want: `{"Mississippi": 5}`,
		},
		{
			name: "set another metric",
			args: args{k: "Nile", v: monitor.Counter(79)},
			want: `{"Nile": 79, "Mississippi": 5}`,
		},
	}

	s, err := New(context.Background(), "postgres://postgres:123456@localhost:5432/database", "", 5, false)
	if assert.NoError(t, err) {
		for _, tt := range tests {
			t.Run(tt.name, func(t *testing.T) {
				s.AddCounter(context.TODO(), tt.args.k, tt.args.v)
				counterJSON, err := s.StringCounter(context.TODO())
				assert.NoError(t, err)
				assert.JSONEq(t, tt.want, counterJSON)
			})
		}
	}
}
