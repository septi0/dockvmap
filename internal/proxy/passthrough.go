package proxy

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net"
	"net/http"
	"net/http/httputil"
	"net/url"
	"strconv"
	"strings"
	"time"

	"github.com/septi0/dockvmap/internal/service"
)

const (
	RequestReadTimeout = 15 * time.Second

	passthroughDialTimeout           = 5 * time.Second
	passthroughTLSHandshakeTimeout   = 10 * time.Second
	passthroughResponseHeaderTimeout = 2 * time.Minute
	passthroughIdleConns             = 16
)

var (
	readTimeout           = RequestReadTimeout
	uploadIdleTimeout     = time.Minute
	uploadDeadlineRefresh = time.Second
)

type passthrough struct {
	address string
	prefix  string
	changes changeRecorder
	proxy   *httputil.ReverseProxy
}

type change struct {
	repository, reference, token string
	deleted                      bool
	info                         service.RequestInfo
}

type changeKey struct{}

func withChange(ctx context.Context, c change) context.Context {
	return context.WithValue(ctx, changeKey{}, c)
}

func newPassthrough(address string, useTLS bool, changes changeRecorder) *passthrough {
	target := &url.URL{Scheme: "http", Host: address}

	if useTLS {
		target.Scheme = "https"
	}

	pt := &passthrough{address: address, prefix: target.String(), changes: changes}

	pt.proxy = &httputil.ReverseProxy{
		Rewrite: func(pr *httputil.ProxyRequest) {
			pr.SetURL(target)
			pr.Out.Header.Del("Authorization")
		},
		ModifyResponse: pt.modifyResponse,
		ErrorHandler:   pt.errorHandler,
		ErrorLog:       slog.NewLogLogger(slog.Default().Handler(), slog.LevelDebug),
		FlushInterval:  -1,
		Transport: &http.Transport{
			DialContext:           (&net.Dialer{Timeout: passthroughDialTimeout}).DialContext,
			TLSHandshakeTimeout:   passthroughTLSHandshakeTimeout,
			ResponseHeaderTimeout: passthroughResponseHeaderTimeout,
			MaxIdleConnsPerHost:   passthroughIdleConns,
			IdleConnTimeout:       90 * time.Second,
		},
	}

	return pt
}

type uploadBody struct {
	io.ReadCloser
	controller *http.ResponseController
	extended   time.Time
}

func (b *uploadBody) Read(p []byte) (int, error) {
	if now := time.Now(); now.Sub(b.extended) > uploadDeadlineRefresh {
		b.extended = now

		if err := b.controller.SetReadDeadline(now.Add(uploadIdleTimeout)); err != nil {
			slog.Debug("extending the upload read deadline failed", "error", err)
		}
	}

	return b.ReadCloser.Read(p)
}

func setReadDeadline(w http.ResponseWriter) *http.ResponseController {
	controller := http.NewResponseController(w)

	if err := controller.SetReadDeadline(time.Now().Add(readTimeout)); err != nil {
		slog.Debug("setting the request read deadline failed", "error", err)
	}

	return controller
}

func (pt *passthrough) modifyResponse(resp *http.Response) error {
	if location := resp.Header.Get("Location"); strings.HasPrefix(location, pt.prefix+"/") {
		resp.Header.Set("Location", strings.TrimPrefix(location, pt.prefix))
	}

	if c, ok := resp.Request.Context().Value(changeKey{}).(change); ok && pt.changes != nil {
		switch {
		case !c.deleted && resp.StatusCode == http.StatusCreated:
			pt.changes.Pushed(c.repository, c.reference, c.token, c.info)
		case c.deleted && resp.StatusCode == http.StatusAccepted:
			pt.changes.Deleted(c.repository, c.reference, c.token, c.info)
		}
	}

	if resp.StatusCode == http.StatusUnauthorized {
		slog.Error("the internal registry asks for a login", "registry", pt.address, "path", resp.Request.URL.Path)
		replaceWithOCIError(resp, http.StatusBadGateway, "UNAUTHORIZED", "the internal registry asks for a login; remove its htpasswd settings, DockVMap handles logins")
	}

	return nil
}

func (pt *passthrough) errorHandler(w http.ResponseWriter, r *http.Request, err error) {
	upload := r.Method != http.MethodGet && r.Method != http.MethodHead && r.Body != nil && r.Body != http.NoBody

	if upload && r.Context().Err() != nil {
		slog.Info("an upload to the internal registry ended before it finished", "path", r.URL.Path)
		writeOCIError(w, http.StatusRequestTimeout, "BLOB_UPLOAD_INVALID", "the upload stopped sending data and was cut off; push again")
		return
	}

	if r.Context().Err() != nil {
		slog.Debug("client went away during an internal registry request", "path", r.URL.Path, "error", err)
		w.WriteHeader(http.StatusBadGateway)
		return
	}

	if problem := service.InternalRegistryCertProblem(err); problem != nil {
		slog.Warn("the internal registry's certificate isn't accepted", "registry", pt.address, "path", r.URL.Path, "error", problem)
		writeOCIError(w, http.StatusBadGateway, "UNKNOWN", fmt.Sprintf("the internal registry at %s has a certificate DockVMap doesn't accept: %v", pt.address, problem))

		return
	}

	if isDialError(err) {
		slog.Warn("the internal registry didn't answer", "registry", pt.address, "path", r.URL.Path, "error", err)
		writeOCIError(w, http.StatusBadGateway, "UNKNOWN", internalRegistryUnreachable(pt.address))

		return
	}

	slog.Warn("the internal registry ended the request early", "registry", pt.address, "path", r.URL.Path, "error", err)
	writeOCIError(w, http.StatusBadGateway, "UNKNOWN", fmt.Sprintf("the internal registry at %s ended the request early; check the registry's log", pt.address))
}

func isDialError(err error) bool {
	var opErr *net.OpError

	return errors.As(err, &opErr) && opErr.Op == "dial"
}

func internalRegistryUnreachable(address string) string {
	return fmt.Sprintf("the internal registry at %s didn't answer; check that its container is running on DockVMap's network", address)
}

func replaceWithOCIError(resp *http.Response, status int, code, message string) {
	body, _ := json.Marshal(ociError{Errors: []ociErrorDetail{{Code: code, Message: message}}})

	resp.Body.Close()
	resp.Body = io.NopCloser(bytes.NewReader(body))
	resp.ContentLength = int64(len(body))
	resp.StatusCode = status
	resp.Status = fmt.Sprintf("%d %s", status, http.StatusText(status))
	resp.Header.Del("Www-Authenticate")
	resp.Header.Set("Content-Type", "application/json")
	resp.Header.Set("Content-Length", strconv.Itoa(len(body)))
}
