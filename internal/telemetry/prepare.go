package telemetry

import (
	"context"
	"strconv"
	"time"

	"github.com/shirou/gopsutil/v3/cpu"
	"github.com/shirou/gopsutil/v3/mem"

	monitor "github.com/a-tho/monitor/internal"
	"github.com/a-tho/monitor/internal/server"
)

func (o *Observer) prepare(ctx context.Context, toReport chan<- []*monitor.Metrics) error {
	var metrics []*monitor.Metrics

	// Add polled metrics (gauge)
	for _, instance := range o.polled {
		// Gauge metrics
		for key, val := range instance.Gauges {
			valFloat := float64(val)
			metric := monitor.Metrics{
				ID:    key,
				MType: server.GaugePath,
				Value: &valFloat,
			}
			metrics = append(metrics, &metric)

		}
	}

	// Add poll count metric (counter)
	delta := int64(o.reportStep)
	metrics = append(metrics,
		&monitor.Metrics{ID: "PollCount", MType: server.CounterPath, Delta: &delta},
	)

	// Add memory usage metrics (gauge)
	virtMem, err := mem.VirtualMemory()
	if err != nil {
		return err
	}
	totalMem, freeMem := float64(virtMem.Total), float64(virtMem.Free)
	metrics = append(metrics,
		&monitor.Metrics{ID: "TotalMemory", MType: server.GaugePath, Value: &totalMem},
		&monitor.Metrics{ID: "FreeMemory", MType: server.GaugePath, Value: &freeMem},
	)

	// Add CPU usage metrics (gauge)
	cpuUtils, err := cpu.PercentWithContext(ctx, time.Microsecond, true)
	if err != nil {
		return err
	}
	for i, util := range cpuUtils {
		util := util
		id := "CPUutilization" + strconv.Itoa(i)
		metrics = append(metrics,
			&monitor.Metrics{ID: id, MType: server.GaugePath, Value: &util},
		)
	}

	// Send prepared metrics batch to worker pool
	toReport <- metrics

	return nil
}
