// Package server implements a multiplexer and handlers necessary for
// processing incoming requests.
package server

import (
	"encoding/base64"
	"fmt"

	"github.com/go-chi/chi/v5"

	monitor "github.com/a-tho/monitor/internal"
	mw "github.com/a-tho/monitor/internal/middleware"
)

type server struct {
	metrics monitor.MetricRepo
}

// NewServer creates a new multiplexer with configured handlers
func NewServer(
	metrics monitor.MetricRepo,
	signKeyStr string,
) *chi.Mux {
	srv := server{metrics: metrics}
	mux := chi.NewRouter()

	signKey, err := base64.StdEncoding.DecodeString(signKeyStr)
	if err != nil {
		signKey = []byte{}
	}

	mux.Get("/", mw.WithLogging(mw.WithSigning(mw.WithCompressing(srv.All), signKey)))

	path := fmt.Sprintf("/%s/{%s}/{%s}/{%s}", UpdPath, TypePath, NamePath, ValuePath)
	mux.Post(path, mw.WithLogging(srv.UpdateLegacy))

	path = fmt.Sprintf("/%s/", UpdPath)
	mux.Post(path, mw.WithLogging(mw.WithSigning(mw.WithCompressing(srv.Update), signKey)))

	path = fmt.Sprintf("/%s/", UpdsPath)
	mux.Post(path, mw.WithLogging(mw.WithSigning(mw.WithCompressing(srv.Updates), signKey)))

	path = fmt.Sprintf("/%s/{%s}/{%s}", ValuePath, TypePath, NamePath)
	mux.Get(path, mw.WithLogging(srv.ValueLegacy))

	path = fmt.Sprintf("/%s/", ValuePath)
	mux.Post(path, mw.WithLogging(mw.WithSigning(mw.WithCompressing(srv.Value), signKey)))

	path = "/ping"
	mux.Get(path, mw.WithLogging(mw.WithSigning(mw.WithCompressing(srv.Ping), signKey)))

	return mux
}

const (
	UpdPath  = "update"
	UpdsPath = "updates"

	GaugePath   = "gauge"
	CounterPath = "counter"

	TypePath  = "type"
	NamePath  = "name"
	ValuePath = "value"
)
