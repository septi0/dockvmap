package httpmw

import (
	"context"
	"io"
	"log/slog"
	"net/http"
	"time"
)

type AccessFields struct {
	VirtualImage     string
	Registry         string
	Repository       string
	Reference        string
	UpstreamRef      string
	Resource         string // "manifest" | "blob"
	Cache            string // "hit" | "miss"
	Authenticated    bool
	AuthenticateSeen bool
}

type accessFieldsKey struct{}

// AccessFieldsFrom returns nil when the AccessLog middleware is not installed.
func AccessFieldsFrom(ctx context.Context) *AccessFields {
	fields, _ := ctx.Value(accessFieldsKey{}).(*AccessFields)
	return fields
}

// AccessLog emits one structured slog line per request, tagged with component.
func AccessLog(component string, next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		start := time.Now()

		fields := &AccessFields{}
		r = r.WithContext(context.WithValue(r.Context(), accessFieldsKey{}, fields))

		sw := &accessWriter{ResponseWriter: w, status: http.StatusOK}

		defer func() {
			attrs := []any{
				slog.String("component", component),
				slog.String("method", r.Method),
				slog.String("path", r.URL.Path),
				slog.Int("status", sw.status),
				slog.Int64("bytes", sw.bytes),
				slog.Int64("duration_ms", time.Since(start).Milliseconds()),
				slog.String("remote_addr", r.RemoteAddr),
			}

			if xff := r.Header.Get("X-Forwarded-For"); xff != "" {
				attrs = append(attrs, slog.String("forwarded_for", xff))
			}

			if fields.VirtualImage != "" {
				attrs = append(attrs, slog.String("virtual_image", fields.VirtualImage))
			}

			if fields.Registry != "" {
				attrs = append(attrs, slog.String("registry", fields.Registry))
			}

			if fields.Repository != "" {
				attrs = append(attrs, slog.String("repository", fields.Repository))
			}

			if fields.Reference != "" {
				attrs = append(attrs, slog.String("reference", fields.Reference))
			}

			if fields.UpstreamRef != "" && fields.UpstreamRef != fields.Reference {
				attrs = append(attrs, slog.String("upstream_ref", fields.UpstreamRef))
			}

			if fields.Resource != "" {
				attrs = append(attrs, slog.String("resource", fields.Resource))
			}

			if fields.Cache != "" {
				attrs = append(attrs, slog.String("cache", fields.Cache))
			}

			if fields.AuthenticateSeen {
				attrs = append(attrs, slog.Bool("authenticated", fields.Authenticated))
			}

			slog.Info("proxy request", attrs...)
		}()

		next.ServeHTTP(sw, r)
	})
}

type accessWriter struct {
	http.ResponseWriter
	status      int
	bytes       int64
	wroteHeader bool
}

func (w *accessWriter) WriteHeader(status int) {
	if !w.wroteHeader {
		w.status = status
		w.wroteHeader = true
	}

	w.ResponseWriter.WriteHeader(status)
}

func (w *accessWriter) Write(b []byte) (int, error) {
	w.wroteHeader = true

	n, err := w.ResponseWriter.Write(b)
	w.bytes += int64(n)

	return n, err
}

func (w *accessWriter) Unwrap() http.ResponseWriter {
	return w.ResponseWriter
}

func (w *accessWriter) Flush() {
	if f, ok := w.ResponseWriter.(http.Flusher); ok {
		f.Flush()
	}
}

func (w *accessWriter) ReadFrom(src io.Reader) (int64, error) {
	w.wroteHeader = true

	var n int64
	var err error

	if rf, ok := w.ResponseWriter.(io.ReaderFrom); ok {
		n, err = rf.ReadFrom(src)
	} else {
		n, err = io.Copy(w.ResponseWriter, src)
	}

	w.bytes += n

	return n, err
}
