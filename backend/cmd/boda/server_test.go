package main

import (
	"net/http"
	"testing"
	"time"
)

func TestNewServerSetsConnectionLimitsWithoutTotalTimeouts(t *testing.T) {
	handler := http.NewServeMux()
	server := newServer(":8080", handler)

	if server.Addr != ":8080" || server.Handler != handler {
		t.Errorf("newServer() addr/handler = %q/%v; want the given address and handler", server.Addr, server.Handler)
	}
	if server.ReadHeaderTimeout != 5*time.Second {
		t.Errorf("ReadHeaderTimeout = %s; want 5s", server.ReadHeaderTimeout)
	}
	if server.IdleTimeout != 120*time.Second {
		t.Errorf("IdleTimeout = %s; want 120s", server.IdleTimeout)
	}
	if server.MaxHeaderBytes != 64<<10 {
		t.Errorf("MaxHeaderBytes = %d; want 65536", server.MaxHeaderBytes)
	}
	// Uploads stream for minutes on slow links, so total read and write
	// deadlines must stay unset; the per-read idle timeout protects them.
	if server.ReadTimeout != 0 || server.WriteTimeout != 0 {
		t.Errorf("ReadTimeout/WriteTimeout = %s/%s; want both unset", server.ReadTimeout, server.WriteTimeout)
	}
}
