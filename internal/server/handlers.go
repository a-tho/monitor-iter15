package server

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"strconv"

	"github.com/go-chi/chi/v5"

	monitor "github.com/a-tho/monitor/internal"
)

const (
	batchSize = 1000

	errPostMethod  = "use POST for saving metrics"
	errMetricPath  = "invalid metric path"
	errMetricType  = "invalid metric type"
	errMetricName  = "invalid metric name"
	errMetricValue = "invalid metric value"
	errMetricHTML  = "failed to generate HTML page with metrics"
	errDecompress  = "failed to decompress request body"
	errSetGauge    = "failed to set gauge value"

	// HTML
	metricsTemplate = `
		{{range $key, $value := .}}
			<p>{{$key}}: {{$value}}</p>
		{{end}}`
	pageHead = `
	<!DOCTYPE html>
	<html>
	<head>
		<title>Metrics</title>
	</head>
	<body>`
	gaugeHeader   = `<h1>Gauge metrics</h1>`
	counterHeader = `<h1>Counter metrics</h1>`
	pageFooter    = `
	</body>
	</html>`

	contentType         = "Content-Type"
	contentEncoding     = "Content-Encoding"
	acceptEncoding      = "Accept-Encoding"
	typeApplicationJSON = "application/json"
	typeTextHTML        = "text/html"
	encodingGzip        = "gzip"
)

// UpdateLegacy handles requests for adding metrics
func (s *server) UpdateLegacy(w http.ResponseWriter, r *http.Request) {
	typ := chi.URLParam(r, TypePath)
	name := chi.URLParam(r, NamePath)
	value := chi.URLParam(r, ValuePath)
	if name == "" {
		http.NotFound(w, r)
		return
	}

	switch typ {
	case GaugePath:
		v, err := strconv.ParseFloat(value, 64)
		if err != nil {
			http.Error(w, errMetricValue, http.StatusBadRequest)
			return
		}
		_, err = s.metrics.SetGauge(r.Context(), name, monitor.Gauge(v))
		if err != nil {
			http.Error(w, errSetGauge, http.StatusInternalServerError)
			return
		}
	case CounterPath:
		v, err := strconv.ParseInt(value, 10, 64)
		if err != nil {
			http.Error(w, errMetricValue, http.StatusBadRequest)
			return
		}
		_, err = s.metrics.AddCounter(r.Context(), name, monitor.Counter(v))
		if err != nil {
			http.Error(w, errSetGauge, http.StatusInternalServerError)
			return
		}
	default:
		http.Error(w, errMetricPath, http.StatusBadRequest)
	}
}

func (s *server) Update(w http.ResponseWriter, r *http.Request) {
	if r.Header.Get(contentType) != typeApplicationJSON {
		http.NotFound(w, r)
		return
	}

	var input monitor.Metrics
	dec := json.NewDecoder(r.Body)
	err := dec.Decode(&input)
	if err != nil {
		http.Error(w, errMetricValue, http.StatusBadRequest)
		return
	}

	var respValue float64
	switch input.MType {
	case GaugePath:

		if input.Value == nil {
			http.Error(w, errMetricValue, http.StatusBadRequest)
			return
		}
		_, err = s.metrics.SetGauge(r.Context(), input.ID, monitor.Gauge(*input.Value))
		if err != nil {
			http.Error(w, errSetGauge, http.StatusInternalServerError)
			return
		}

		respValue = *input.Value

	case CounterPath:

		if input.Delta == nil {
			http.Error(w, errMetricValue, http.StatusBadRequest)
			return
		}
		_, err = s.metrics.AddCounter(r.Context(), input.ID, monitor.Counter(*input.Delta))
		if err != nil {
			http.Error(w, errSetGauge, http.StatusInternalServerError)
			return
		}

		input.Delta = nil
		counter, _ := s.metrics.GetCounter(r.Context(), input.ID)
		respValue = float64(counter)

	default:
		http.Error(w, errMetricType, http.StatusBadRequest)
		return
	}

	input.Value = &respValue
	w.Header().Add(contentType, typeApplicationJSON)
	enc := json.NewEncoder(w)
	enc.Encode(input)
}

func (s *server) Updates(w http.ResponseWriter, r *http.Request) {
	if r.Header.Get(contentType) != typeApplicationJSON {
		http.NotFound(w, r)
		return
	}

	dec := json.NewDecoder(r.Body)

	token, err := dec.Token()
	if err != nil || token.(json.Delim) != '[' {
		http.Error(w, errMetricValue, http.StatusBadRequest)
		return
	}

	batchGauge := make([]*monitor.Metrics, 0, batchSize)
	batchCounter := make([]*monitor.Metrics, 0, batchSize)
	for dec.More() {
		metric := &monitor.Metrics{}
		if err = dec.Decode(metric); err != nil {
			http.Error(w, errMetricValue, http.StatusBadRequest)
			return
		}

		switch metric.MType {
		case GaugePath:
			if metric.Value == nil {
				http.Error(w, errMetricValue, http.StatusBadRequest)
				return
			}
			batchGauge = append(batchGauge, metric)
			if len(batchGauge) >= batchSize {
				_, err = s.metrics.SetGaugeBatch(r.Context(), batchGauge)
				if err != nil {
					http.Error(w, errMetricValue, http.StatusBadRequest)
					return
				}
				batchGauge = batchGauge[:0]
			}

		case CounterPath:
			if metric.Delta == nil {
				http.Error(w, errMetricValue, http.StatusBadRequest)
				return
			}
			batchCounter = append(batchCounter, metric)
			if len(batchCounter) >= batchSize {
				_, err = s.metrics.AddCounterBatch(r.Context(), batchCounter)
				if err != nil {
					http.Error(w, errMetricValue, http.StatusBadRequest)
					return
				}
				batchCounter = batchCounter[:0]
			}

		default:
			http.Error(w, errMetricType, http.StatusBadRequest)
			return
		}
	}

	if len(batchGauge) > 0 {
		s.metrics.SetGaugeBatch(r.Context(), batchGauge)
	}
	if len(batchCounter) > 0 {
		s.metrics.AddCounterBatch(r.Context(), batchCounter)
	}
}

func (s *server) ValueLegacy(w http.ResponseWriter, r *http.Request) {
	typ := chi.URLParam(r, "type")
	name := chi.URLParam(r, "name")

	switch typ {
	case GaugePath:
		value, ok := s.metrics.GetGauge(r.Context(), name)
		if !ok {
			http.NotFound(w, r)
			return
		}
		v := strconv.FormatFloat(float64(value), 'f', -1, 64)
		w.Write([]byte(v))
	case CounterPath:
		value, ok := s.metrics.GetCounter(r.Context(), name)
		if !ok {
			http.NotFound(w, r)
			return
		}
		v := strconv.FormatInt(int64(value), 10)
		w.Write([]byte(v))
	default:
		http.Error(w, errMetricPath, http.StatusBadRequest)
	}
}

func (s *server) Value(w http.ResponseWriter, r *http.Request) {
	if r.Header.Get(contentType) != typeApplicationJSON {
		http.NotFound(w, r)
		return
	}

	var input monitor.Metrics
	dec := json.NewDecoder(r.Body)
	err := dec.Decode(&input)
	if err != nil {
		http.Error(w, errMetricValue, http.StatusBadRequest)
		return
	}

	if input.ID == "" {
		http.Error(w, errMetricName, http.StatusBadRequest)
		return
	}
	switch input.MType {
	case GaugePath:

		val, ok := s.metrics.GetGauge(r.Context(), input.ID)
		if !ok {
			http.NotFound(w, r)
			return
		}
		valFloat := float64(val)
		input.Value = &valFloat

	case CounterPath:

		count, ok := s.metrics.GetCounter(r.Context(), input.ID)
		if !ok {
			http.NotFound(w, r)
			return
		}
		countInt := int64(count)
		input.Delta = &countInt

	default:
		http.Error(w, errMetricType, http.StatusBadRequest)
		return
	}

	w.Header().Add(contentType, typeApplicationJSON)
	enc := json.NewEncoder(w)
	enc.Encode(input)
}

func (s *server) All(w http.ResponseWriter, r *http.Request) {
	var gaugeBuf bytes.Buffer
	if err := s.metrics.WriteAllGauge(r.Context(), &gaugeBuf); err != nil {
		http.Error(w, errMetricHTML, http.StatusInternalServerError)
		return
	}

	var counterBuf bytes.Buffer
	if err := s.metrics.WriteAllCounter(r.Context(), &counterBuf); err != nil {
		http.Error(w, errMetricHTML, http.StatusInternalServerError)
		return
	}

	w.Header().Add(contentType, typeTextHTML)
	w.Write([]byte(pageHead))
	w.Write([]byte(gaugeHeader))
	w.Write(gaugeBuf.Bytes())
	w.Write([]byte(counterHeader))
	w.Write(counterBuf.Bytes())
	w.Write([]byte(pageFooter))
}

func (s *server) Ping(w http.ResponseWriter, r *http.Request) {
	if err := s.metrics.PingContext(context.TODO()); err != nil {
		http.Error(w, "ping unsuccessful", http.StatusInternalServerError)
	}
	w.Write(nil)
}
