package middleware

import (
	"net/http"
	"time"

	"github.com/rs/zerolog/log"
)

type (
	respData struct {
		code int
		size int
	}

	logResponseWriter struct {
		http.ResponseWriter
		data *respData
	}
)

func (w *logResponseWriter) Write(data []byte) (int, error) {
	size, err := w.ResponseWriter.Write(data)
	w.data.size += size
	return size, err
}

func (w *logResponseWriter) WriteHeader(code int) {
	w.ResponseWriter.WriteHeader(code)
	w.data.code = code
}

func WithLogging(handler func(w http.ResponseWriter, r *http.Request)) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		start := time.Now()

		respData := respData{code: 200}

		lw := logResponseWriter{ResponseWriter: w, data: &respData}

		log.Info().Str("uri", r.RequestURI).Msg("")
		log.Info().Str("method", r.Method).Msg("")

		handler(&lw, r)

		duration := time.Since(start)

		log.Info().Dur("duration", duration).Msg("")
		log.Info().Int("code", respData.code).Msg("")
		log.Info().Int("size", respData.size).Msg("")
	}
}
