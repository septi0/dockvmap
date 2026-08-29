// Package httpmw holds HTTP middleware shared by the web and proxy servers.
package httpmw

import (
	"io"
	"log/slog"
	"net/http"
	"runtime/debug"
)

// Recover turns a handler panic into a logged 500 instead of a dropped connection.
func Recover(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		rw := &recoverWriter{ResponseWriter: w}

		defer func() {
			recovered := recover()

			if recovered == nil {
				return
			}

			slog.Error("panic serving request",
				"method", r.Method,
				"path", r.URL.Path,
				"remote", r.RemoteAddr,
				"panic", recovered,
				"stack", string(debug.Stack()),
			)

			if !rw.started {
				w.Header().Set("Content-Type", "text/plain; charset=utf-8")
				w.WriteHeader(http.StatusInternalServerError)
				_, _ = io.WriteString(w, "internal server error\n")
			}
		}()

		next.ServeHTTP(rw, r)
	})
}

type recoverWriter struct {
	http.ResponseWriter
	started bool
}

func (w *recoverWriter) WriteHeader(status int) {
	w.started = true
	w.ResponseWriter.WriteHeader(status)
}

func (w *recoverWriter) Write(b []byte) (int, error) {
	w.started = true
	return w.ResponseWriter.Write(b)
}

func (w *recoverWriter) Unwrap() http.ResponseWriter {
	return w.ResponseWriter
}

func (w *recoverWriter) Flush() {
	if f, ok := w.ResponseWriter.(http.Flusher); ok {
		w.started = true
		f.Flush()
	}
}

func (w *recoverWriter) ReadFrom(src io.Reader) (int64, error) {
	w.started = true

	if rf, ok := w.ResponseWriter.(io.ReaderFrom); ok {
		return rf.ReadFrom(src)
	}

	return io.Copy(w.ResponseWriter, src)
}
