package telemetry

import (
	"math/rand"
	"runtime"
	"time"

	monitor "github.com/a-tho/monitor/internal"
)

func (o *Observer) poll(pollCount int) {
	var (
		countSinceReport = pollCount % o.reportStep // the number of polls since the last report
		memStats         runtime.MemStats
	)
	runtime.ReadMemStats(&memStats)
	o.polled[countSinceReport].Gauges["Alloc"] = monitor.Gauge(memStats.Alloc)
	o.polled[countSinceReport].Gauges["BuckHashSys"] = monitor.Gauge(memStats.BuckHashSys)
	o.polled[countSinceReport].Gauges["Frees"] = monitor.Gauge(memStats.Frees)
	o.polled[countSinceReport].Gauges["GCCPUFraction"] = monitor.Gauge(memStats.GCCPUFraction)
	o.polled[countSinceReport].Gauges["GCSys"] = monitor.Gauge(memStats.GCSys)
	o.polled[countSinceReport].Gauges["HeapAlloc"] = monitor.Gauge(memStats.HeapAlloc)
	o.polled[countSinceReport].Gauges["HeapIdle"] = monitor.Gauge(memStats.HeapIdle)
	o.polled[countSinceReport].Gauges["HeapInuse"] = monitor.Gauge(memStats.HeapInuse)
	o.polled[countSinceReport].Gauges["HeapObjects"] = monitor.Gauge(memStats.HeapObjects)
	o.polled[countSinceReport].Gauges["HeapReleased"] = monitor.Gauge(memStats.HeapReleased)
	o.polled[countSinceReport].Gauges["HeapSys"] = monitor.Gauge(memStats.HeapSys)
	o.polled[countSinceReport].Gauges["LastGC"] = monitor.Gauge(memStats.LastGC)
	o.polled[countSinceReport].Gauges["Lookups"] = monitor.Gauge(memStats.Lookups)
	o.polled[countSinceReport].Gauges["MCacheInuse"] = monitor.Gauge(memStats.MCacheInuse)
	o.polled[countSinceReport].Gauges["MCacheSys"] = monitor.Gauge(memStats.MCacheSys)
	o.polled[countSinceReport].Gauges["MSpanInuse"] = monitor.Gauge(memStats.MSpanInuse)
	o.polled[countSinceReport].Gauges["MSpanSys"] = monitor.Gauge(memStats.MSpanSys)
	o.polled[countSinceReport].Gauges["Mallocs"] = monitor.Gauge(memStats.Mallocs)
	o.polled[countSinceReport].Gauges["NextGC"] = monitor.Gauge(memStats.NextGC)
	o.polled[countSinceReport].Gauges["NumForcedGC"] = monitor.Gauge(memStats.NumForcedGC)
	o.polled[countSinceReport].Gauges["NumGC"] = monitor.Gauge(memStats.NumGC)
	o.polled[countSinceReport].Gauges["OtherSys"] = monitor.Gauge(memStats.OtherSys)
	o.polled[countSinceReport].Gauges["PauseTotalNs"] = monitor.Gauge(memStats.PauseTotalNs)
	o.polled[countSinceReport].Gauges["StackInuse"] = monitor.Gauge(memStats.StackInuse)
	o.polled[countSinceReport].Gauges["StackSys"] = monitor.Gauge(memStats.StackSys)
	o.polled[countSinceReport].Gauges["Sys"] = monitor.Gauge(memStats.Sys)
	o.polled[countSinceReport].Gauges["TotalAlloc"] = monitor.Gauge(memStats.TotalAlloc)

	randomValue := rand.New(rand.NewSource(time.Now().Unix())).Float64()
	o.polled[countSinceReport].Gauges["RandomValue"] = monitor.Gauge(randomValue)
}
