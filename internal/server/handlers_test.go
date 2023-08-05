package server

import (
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	monitor "github.com/a-tho/monitor/internal"
	"github.com/a-tho/monitor/internal/storage"
)

const (
	notFoundResponse = "404 page not found\n"

	textPlain = "text/plain; charset=utf-8"
)

type request struct {
	method string
	path   string
}

type want struct {
	// Response-related fields
	code        int
	respBody    string
	contentType string

	// Storage-related fields
	gauge   string
	counter string
}

type state struct {
	metrics monitor.MetricRepo
}

func testRequest(t *testing.T, srv *httptest.Server, method, path string, body io.Reader) (*http.Response, string) {
	req, err := http.NewRequest(method, srv.URL+path, body)
	require.NoError(t, err)

	resp, err := srv.Client().Do(req)
	require.NoError(t, err)

	respBody, err := io.ReadAll(resp.Body)
	require.NoError(t, err)

	return resp, string(respBody)
}

func TestServerUpdHandler(t *testing.T) {
	tests := []struct {
		name    string
		request request
		want    want
	}{
		{
			name: "invalid request method",
			request: request{
				method: http.MethodGet,
				path:   "/" + UpdPath + "/" + GaugePath + "/" + "Apple" + "/" + "3",
			},
			want: want{
				code:        http.StatusMethodNotAllowed,
				respBody:    "",
				contentType: "",
				gauge:       `{}`,
				counter:     `{}`,
			},
		},
		{
			name: "no metric name",
			request: request{
				method: http.MethodPost,
				path:   "/" + UpdPath,
			},
			want: want{
				code:        http.StatusNotFound,
				respBody:    notFoundResponse,
				contentType: textPlain,
				gauge:       `{}`,
				counter:     `{}`,
			},
		},
		{
			name: "wrong metric type",
			request: request{
				method: http.MethodPost,
				path:   "/" + UpdPath + "/" + "wrongtype" + "/" + "Apple" + "/" + "3",
			},
			want: want{
				code:        http.StatusBadRequest,
				respBody:    errMetricPath + "\n",
				contentType: textPlain,
				gauge:       `{}`,
				counter:     `{}`,
			},
		},
		{
			name: "wrong metric value for counter",
			request: request{
				method: http.MethodPost,
				path:   "/" + UpdPath + "/" + CounterPath + "/" + "Apple" + "/" + "wrongvalue",
			},
			want: want{
				code:        http.StatusBadRequest,
				respBody:    errMetricValue + "\n",
				contentType: textPlain,
				gauge:       `{}`,
				counter:     `{}`,
			},
		},
		{
			name: "valid gauge request",
			request: request{
				method: http.MethodPost,
				path:   "/" + UpdPath + "/" + GaugePath + "/" + "Apple" + "/" + "3",
			},
			want: want{
				code:        http.StatusOK,
				respBody:    "",
				contentType: "",
				gauge:       `{"Apple": 3}`,
				counter:     "{}",
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			metrics, err := storage.New(context.Background(), "temporary_stub", "", 5, false)
			if assert.NoError(t, err) {
				srv := httptest.NewServer(NewServer(metrics, ""))
				defer srv.Close()

				resp, respBody := testRequest(t, srv, tt.request.method, tt.request.path, nil)
				defer resp.Body.Close()

				// Validate response
				assert.Equal(t, tt.want.code, resp.StatusCode)
				assert.Equal(t, tt.want.contentType, resp.Header.Get("Content-Type"))
				assert.Equal(t, tt.want.respBody, string(respBody))

				// Validate server storage
				gaugeJSON, err := metrics.StringGauge(context.TODO())
				assert.NoError(t, err)
				assert.JSONEq(t, tt.want.gauge, gaugeJSON)
				counterJSON, err := metrics.StringCounter(context.TODO())
				assert.NoError(t, err)
				assert.JSONEq(t, tt.want.counter, counterJSON)
			}
		})
	}
}

// func TestGetValHandler(t *testing.T) {
// 	// I don't know what the best practices for initializing exernal storage is
// 	// so I updated storage interface methods for modifying it: now they return
// 	// the storage, so that I can chain several storage modification operations
// 	// in one line (see the line for initializing the gauge storage below)

// 	tests := []struct {
// 		name    string
// 		request request
// 		want    want
// 		state   state
// 	}{
// 		{
// 			name: "no such metric name",
// 			request: request{
// 				method: http.MethodGet,
// 				path:   "/" + ValuePath + "/" + GaugePath + "/" + "Apple",
// 			},
// 			want: want{
// 				code:        http.StatusNotFound,
// 				respBody:    notFoundResponse,
// 				contentType: textPlain,
// 			},
// 			state: state{
// 				metrics: storage.New("", 5, false).SetGauge("Peach", monitor.Gauge(4.0)),
// 			},
// 		},
// 		{
// 			name: "metric value is present",
// 			request: request{
// 				method: http.MethodGet,
// 				path:   "/" + ValuePath + "/" + GaugePath + "/" + "Apple",
// 			},
// 			want: want{
// 				code:        http.StatusOK,
// 				respBody:    "20",
// 				contentType: textPlain,
// 			},
// 			state: state{
// 				metrics: storage.New("", 5, false).SetGauge("Apple", monitor.Gauge(20.0)),
// 			},
// 		},
// 	}

// 	for _, tt := range tests {
// 		t.Run(tt.name, func(t *testing.T) {
// 			srv := httptest.NewServer(NewServer(tt.state.metrics))
// 			defer srv.Close()

// 			resp, respBody := testRequest(t, srv, tt.request.method, tt.request.path, nil)
// 			defer resp.Body.Close()

// 			// Validate response
// 			assert.Equal(t, tt.want.code, resp.StatusCode)
// 			assert.Equal(t, tt.want.contentType, resp.Header.Get("Content-Type"))
// 			assert.Equal(t, tt.want.respBody, respBody)
// 		})
// 	}
// }
