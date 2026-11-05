package main

import (
	"errors"
	"net/http"
	"testing"
)

func TestAwaitShutdownReturnsTheServerError(t *testing.T) {
	failed := errors.New("web server: listen tcp 127.0.0.1:8080: bind: address already in use")
	serverErrs := make(chan error, 1)
	serverErrs <- failed

	workerDone := make(chan struct{})
	close(workerDone)

	err := awaitShutdown(&http.Server{}, &http.Server{}, func() {}, workerDone, serverErrs)

	if !errors.Is(err, failed) {
		t.Errorf("awaitShutdown() = %v, want the server error so the process exits non-zero", err)
	}
}
