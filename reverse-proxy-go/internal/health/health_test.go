package health

import (
	"net/http"
	"net/http/httptest"
	"sync"
	"testing"
	"time"
)

func TestRegisterUpstream(t *testing.T) {
	hc := NewHealthChecker()
	defer hc.Close()

	hc.RegisterUpstream("http://localhost:3000", "/health", 100)

	status := hc.IsHealthy("http://localhost:3000")
	if !status {
		t.Fatalf("expected newly registered upstream to be healthy")
	}
}

func TestHealthCheck(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	defer server.Close()

	hc := NewHealthChecker()
	defer hc.Close()

	hc.RegisterUpstream(server.URL, "/health", 50)

	time.Sleep(100 * time.Millisecond)

	if !hc.IsHealthy(server.URL) {
		t.Fatalf("expected healthy upstream")
	}
}

func TestUnhealthyUpstream(t *testing.T) {
	hc := NewHealthChecker()
	defer hc.Close()

	hc.RegisterUpstream("http://localhost:9999", "/health", 50)

	time.Sleep(150 * time.Millisecond)

	if hc.IsHealthy("http://localhost:9999") {
		t.Fatalf("expected unhealthy upstream")
	}
}

func TestHealthCheckStatusCodes(t *testing.T) {
	tests := []struct {
		name       string
		statusCode int
		expected   bool
	}{
		{"200 OK", http.StatusOK, true},
		{"201 Created", http.StatusCreated, true},
		{"204 No Content", http.StatusNoContent, true},
		{"301 Moved", http.StatusMovedPermanently, true},
		{"302 Found", http.StatusFound, true},
		{"400 Bad Request", http.StatusBadRequest, false},
		{"404 Not Found", http.StatusNotFound, false},
		{"500 Internal Error", http.StatusInternalServerError, false},
		{"503 Service Unavailable", http.StatusServiceUnavailable, false},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				w.WriteHeader(test.statusCode)
			}))
			defer server.Close()

			hc := NewHealthChecker()
			defer hc.Close()

			hc.RegisterUpstream(server.URL, "/health", 50)

			time.Sleep(100 * time.Millisecond)

			if hc.IsHealthy(server.URL) != test.expected {
				t.Fatalf("expected healthy=%v for status %d", test.expected, test.statusCode)
			}
		})
	}
}

func TestUnregisterUpstream(t *testing.T) {
	hc := NewHealthChecker()
	defer hc.Close()

	hc.RegisterUpstream("http://localhost:3000", "/health", 100)
	hc.UnregisterUpstream("http://localhost:3000")

	status := hc.GetStatus()
	if len(status) != 0 {
		t.Fatalf("expected no upstreams after unregistering")
	}
}

func TestGetStatus(t *testing.T) {
	server1 := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	defer server1.Close()

	server2 := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
	}))
	defer server2.Close()

	hc := NewHealthChecker()
	defer hc.Close()

	hc.RegisterUpstream(server1.URL, "/health", 50)
	hc.RegisterUpstream(server2.URL, "/health", 50)

	time.Sleep(150 * time.Millisecond)

	status := hc.GetStatus()
	if len(status) != 2 {
		t.Fatalf("expected 2 upstreams in status")
	}

	if !status[server1.URL] {
		t.Fatalf("expected server1 to be healthy")
	}

	if status[server2.URL] {
		t.Fatalf("expected server2 to be unhealthy")
	}
}

func TestIsHealthyUnregistered(t *testing.T) {
	hc := NewHealthChecker()
	defer hc.Close()

	if !hc.IsHealthy("http://localhost:9999") {
		t.Fatalf("expected unregistered upstream to default to healthy")
	}
}

func TestHealthCheckInterval(t *testing.T) {
	var checkCount int
	var mu sync.Mutex

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		mu.Lock()
		checkCount++
		mu.Unlock()
		w.WriteHeader(http.StatusOK)
	}))
	defer server.Close()

	hc := NewHealthChecker()
	defer hc.Close()

	hc.RegisterUpstream(server.URL, "/health", 50)

	time.Sleep(200 * time.Millisecond)

	mu.Lock()
	count := checkCount
	mu.Unlock()

	if count < 3 {
		t.Fatalf("expected at least 3 health checks, got %d", count)
	}
}

func TestMultipleRegistrations(t *testing.T) {
	hc := NewHealthChecker()
	defer hc.Close()

	hc.RegisterUpstream("http://localhost:3000", "/health", 100)
	hc.RegisterUpstream("http://localhost:3000", "/health", 100)

	status := hc.GetStatus()
	if len(status) != 1 {
		t.Fatalf("expected only 1 upstream registered")
	}
}

func TestConcurrentIsHealthy(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	defer server.Close()

	hc := NewHealthChecker()
	defer hc.Close()

	hc.RegisterUpstream(server.URL, "/health", 100)

	done := make(chan bool, 10)
	for i := 0; i < 10; i++ {
		go func() {
			_ = hc.IsHealthy(server.URL)
			done <- true
		}()
	}

	for i := 0; i < 10; i++ {
		<-done
	}
}

func TestClose(t *testing.T) {
	hc := NewHealthChecker()

	hc.RegisterUpstream("http://localhost:3000", "/health", 100)
	hc.RegisterUpstream("http://localhost:4000", "/health", 100)

	hc.Close()

	status := hc.GetStatus()
	if len(status) != 0 {
		t.Fatalf("expected all upstreams cleared after Close")
	}
}
