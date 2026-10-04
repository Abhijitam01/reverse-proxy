package handler

import (
	"bytes"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"reverse-proxy-go/internal/config"
	"reverse-proxy-go/internal/health"
	"reverse-proxy-go/internal/router"
)

func TestProxyBasicForwarding(t *testing.T) {
	upstreamServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		w.Write([]byte("upstream response"))
	}))
	defer upstreamServer.Close()

	hc := health.NewHealthChecker()
	defer hc.Close()

	r := router.NewRouter(hc)
	h := NewHandler(r, hc)

	r.AddRoute(config.RouteConfig{
		Match: config.MatchConfig{
			PathPrefix: "/api",
		},
		Upstream:    upstreamServer.URL,
		StripPrefix: true,
	})

	req := httptest.NewRequest("GET", "/api/test", nil)
	w := httptest.NewRecorder()

	h.ServeProxy(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected status 200, got %d", w.Code)
	}

	body, _ := io.ReadAll(w.Body)
	if string(body) != "upstream response" {
		t.Fatalf("expected 'upstream response', got %q", string(body))
	}
}

func TestProxyPrefixStripping(t *testing.T) {
	var capturedPath string
	upstreamServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		capturedPath = r.URL.Path
		w.WriteHeader(http.StatusOK)
	}))
	defer upstreamServer.Close()

	hc := health.NewHealthChecker()
	defer hc.Close()

	r := router.NewRouter(hc)
	h := NewHandler(r, hc)

	r.AddRoute(config.RouteConfig{
		Match: config.MatchConfig{
			PathPrefix: "/api",
		},
		Upstream:    upstreamServer.URL,
		StripPrefix: true,
	})

	req := httptest.NewRequest("GET", "/api/users/123", nil)
	w := httptest.NewRecorder()

	h.ServeProxy(w, req)

	if capturedPath != "/users/123" {
		t.Fatalf("expected path '/users/123', got %q", capturedPath)
	}
}

func TestProxyHostMatching(t *testing.T) {
	upstreamServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		w.Write([]byte("matched"))
	}))
	defer upstreamServer.Close()

	hc := health.NewHealthChecker()
	defer hc.Close()

	r := router.NewRouter(hc)
	h := NewHandler(r, hc)

	r.AddRoute(config.RouteConfig{
		Match: config.MatchConfig{
			Host: "api.example.com",
		},
		Upstream: upstreamServer.URL,
	})

	req := httptest.NewRequest("GET", "http://api.example.com/test", nil)
	req.Host = "api.example.com"
	w := httptest.NewRecorder()

	h.ServeProxy(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected status 200, got %d", w.Code)
	}
}

func TestProxyNoMatch(t *testing.T) {
	hc := health.NewHealthChecker()
	defer hc.Close()

	r := router.NewRouter(hc)
	h := NewHandler(r, hc)

	r.AddRoute(config.RouteConfig{
		Match: config.MatchConfig{
			PathPrefix: "/api",
		},
		Upstream: "http://localhost:9999",
	})

	req := httptest.NewRequest("GET", "/other", nil)
	w := httptest.NewRecorder()

	h.ServeProxy(w, req)

	if w.Code != http.StatusNotFound {
		t.Fatalf("expected status 404, got %d", w.Code)
	}
}

func TestProxyUnhealthyUpstream(t *testing.T) {
	hc := health.NewHealthChecker()
	defer hc.Close()

	r := router.NewRouter(hc)
	h := NewHandler(r, hc)

	r.AddRoute(config.RouteConfig{
		Match: config.MatchConfig{
			PathPrefix: "/api",
		},
		Upstream: "http://localhost:9999",
		HealthCheck: &config.HealthCheckConfig{
			Path:       "/health",
			IntervalMs: 100,
		},
	})

	time.Sleep(200 * time.Millisecond)

	req := httptest.NewRequest("GET", "/api/test", nil)
	w := httptest.NewRecorder()

	h.ServeProxy(w, req)

	if w.Code != http.StatusBadGateway {
		t.Fatalf("expected status 502, got %d", w.Code)
	}
}

func TestAddRequestHeaders(t *testing.T) {
	var capturedHeaders http.Header
	upstreamServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		capturedHeaders = r.Header
		w.WriteHeader(http.StatusOK)
	}))
	defer upstreamServer.Close()

	hc := health.NewHealthChecker()
	defer hc.Close()

	r := router.NewRouter(hc)
	h := NewHandler(r, hc)

	r.AddRoute(config.RouteConfig{
		Match: config.MatchConfig{
			PathPrefix: "/api",
		},
		Upstream:    upstreamServer.URL,
		StripPrefix: true,
		AddRequestHeaders: map[string]string{
			"X-Custom-Header": "custom-value",
			"X-Proxy":         "true",
		},
	})

	req := httptest.NewRequest("GET", "/api/test", nil)
	w := httptest.NewRecorder()

	h.ServeProxy(w, req)

	if capturedHeaders.Get("X-Custom-Header") != "custom-value" {
		t.Fatalf("expected X-Custom-Header to be set")
	}
	if capturedHeaders.Get("X-Proxy") != "true" {
		t.Fatalf("expected X-Proxy to be set")
	}
}

func TestAddResponseHeaders(t *testing.T) {
	upstreamServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("X-Original", "original")
		w.WriteHeader(http.StatusOK)
		w.Write([]byte("response"))
	}))
	defer upstreamServer.Close()

	hc := health.NewHealthChecker()
	defer hc.Close()

	r := router.NewRouter(hc)
	h := NewHandler(r, hc)

	r.AddRoute(config.RouteConfig{
		Match: config.MatchConfig{
			PathPrefix: "/",
		},
		Upstream: upstreamServer.URL,
		AddResponseHeaders: map[string]string{
			"X-Added-Header": "added-value",
		},
	})

	req := httptest.NewRequest("GET", "/", nil)
	w := httptest.NewRecorder()

	h.ServeProxy(w, req)

	if w.Header().Get("X-Added-Header") != "added-value" {
		t.Fatalf("expected X-Added-Header to be added")
	}
}

func TestListRoutes(t *testing.T) {
	hc := health.NewHealthChecker()
	defer hc.Close()

	r := router.NewRouter(hc)
	h := NewHandler(r, hc)

	r.AddRoute(config.RouteConfig{
		Match: config.MatchConfig{
			PathPrefix: "/api",
		},
		Upstream:    "http://localhost:3000",
		StripPrefix: true,
	})

	req := httptest.NewRequest("GET", "/_proxy/routes", nil)
	w := httptest.NewRecorder()

	h.ListRoutes(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected status 200, got %d", w.Code)
	}

	var response ListRoutesResponse
	json.NewDecoder(w.Body).Decode(&response)

	if len(response.Routes) != 1 {
		t.Fatalf("expected 1 route, got %d", len(response.Routes))
	}

	if response.Routes[0].Upstream != "http://localhost:3000" {
		t.Fatalf("expected upstream to be http://localhost:3000")
	}
}

func TestAddRouteAPI(t *testing.T) {
	hc := health.NewHealthChecker()
	defer hc.Close()

	r := router.NewRouter(hc)
	h := NewHandler(r, hc)

	routeReq := AddRouteRequest{
		Match: config.MatchConfig{
			PathPrefix: "/users",
		},
		Upstream:    "http://localhost:4000",
		StripPrefix: true,
	}

	body, _ := json.Marshal(routeReq)
	req := httptest.NewRequest("POST", "/_proxy/routes", bytes.NewReader(body))
	w := httptest.NewRecorder()

	h.AddRoute(w, req)

	if w.Code != http.StatusCreated {
		t.Fatalf("expected status 201, got %d", w.Code)
	}

	routes := r.GetRoutes()
	if len(routes) != 1 {
		t.Fatalf("expected 1 route, got %d", len(routes))
	}
}

func TestRemoveRouteAPI(t *testing.T) {
	hc := health.NewHealthChecker()
	defer hc.Close()

	r := router.NewRouter(hc)
	h := NewHandler(r, hc)

	r.AddRoute(config.RouteConfig{
		Match: config.MatchConfig{
			PathPrefix: "/api",
		},
		Upstream: "http://localhost:3000",
	})

	h.router.RemoveRoute(0)
	routes := h.router.GetRoutes()
	if len(routes) != 0 {
		t.Fatalf("expected 0 routes after removal, got %d", len(routes))
	}
}

func TestHealthEndpoint(t *testing.T) {
	hc := health.NewHealthChecker()
	defer hc.Close()

	r := router.NewRouter(hc)
	h := NewHandler(r, hc)

	upstreamServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	defer upstreamServer.Close()

	r.AddRoute(config.RouteConfig{
		Match: config.MatchConfig{
			PathPrefix: "/",
		},
		Upstream: upstreamServer.URL,
		HealthCheck: &config.HealthCheckConfig{
			Path:       "/health",
			IntervalMs: 100,
		},
	})

	time.Sleep(150 * time.Millisecond)

	req := httptest.NewRequest("GET", "/_proxy/health", nil)
	w := httptest.NewRecorder()

	h.Health(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected status 200, got %d", w.Code)
	}

	var response HealthResponse
	json.NewDecoder(w.Body).Decode(&response)

	if response.Status != "healthy" {
		t.Fatalf("expected status 'healthy', got %q", response.Status)
	}
}

func TestProxyContentPassthrough(t *testing.T) {
	testPayload := []byte(`{"test": "data"}`)
	var capturedPayload []byte

	upstreamServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		capturedPayload, _ = io.ReadAll(r.Body)
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		w.Write([]byte(`{"result": "success"}`))
	}))
	defer upstreamServer.Close()

	hc := health.NewHealthChecker()
	defer hc.Close()

	r := router.NewRouter(hc)
	h := NewHandler(r, hc)

	r.AddRoute(config.RouteConfig{
		Match: config.MatchConfig{
			PathPrefix: "/api",
		},
		Upstream: upstreamServer.URL,
	})

	req := httptest.NewRequest("POST", "/api/data", bytes.NewReader(testPayload))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()

	h.ServeProxy(w, req)

	if !bytes.Equal(capturedPayload, testPayload) {
		t.Fatalf("expected payload %q, got %q", testPayload, capturedPayload)
	}

	responseBody, _ := io.ReadAll(w.Body)
	if !bytes.Contains(responseBody, []byte("success")) {
		t.Fatalf("expected response to contain 'success'")
	}
}

func TestProxyRouteOrdering(t *testing.T) {
	callCount := 0

	upstreamServer1 := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		callCount = 1
		w.WriteHeader(http.StatusOK)
		w.Write([]byte("upstream1"))
	}))
	defer upstreamServer1.Close()

	upstreamServer2 := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		callCount = 2
		w.WriteHeader(http.StatusOK)
		w.Write([]byte("upstream2"))
	}))
	defer upstreamServer2.Close()

	hc := health.NewHealthChecker()
	defer hc.Close()

	r := router.NewRouter(hc)
	h := NewHandler(r, hc)

	r.AddRoute(config.RouteConfig{
		Match: config.MatchConfig{
			PathPrefix: "/api",
		},
		Upstream: upstreamServer1.URL,
	})

	r.AddRoute(config.RouteConfig{
		Match: config.MatchConfig{
			PathPrefix: "/",
		},
		Upstream: upstreamServer2.URL,
	})

	req := httptest.NewRequest("GET", "/api/test", nil)
	w := httptest.NewRecorder()

	h.ServeProxy(w, req)

	if callCount != 1 {
		t.Fatalf("expected first route to match, but got callCount=%d", callCount)
	}
}

func TestRemoveRequestHeaders(t *testing.T) {
	var capturedHeaders http.Header
	upstreamServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		capturedHeaders = r.Header
		w.WriteHeader(http.StatusOK)
	}))
	defer upstreamServer.Close()

	hc := health.NewHealthChecker()
	defer hc.Close()

	r := router.NewRouter(hc)
	h := NewHandler(r, hc)

	r.AddRoute(config.RouteConfig{
		Match: config.MatchConfig{
			PathPrefix: "/api",
		},
		Upstream:             upstreamServer.URL,
		RemoveRequestHeaders: []string{"Authorization", "X-Secret"},
	})

	req := httptest.NewRequest("GET", "/api/test", nil)
	req.Header.Set("Authorization", "Bearer token")
	req.Header.Set("X-Secret", "secret-value")
	req.Header.Set("X-Public", "public-value")

	w := httptest.NewRecorder()
	h.ServeProxy(w, req)

	if capturedHeaders.Get("Authorization") != "" {
		t.Fatalf("expected Authorization header to be removed")
	}
	if capturedHeaders.Get("X-Secret") != "" {
		t.Fatalf("expected X-Secret header to be removed")
	}
	if capturedHeaders.Get("X-Public") == "" {
		t.Fatalf("expected X-Public header to be present")
	}
}

func TestRemoveResponseHeaders(t *testing.T) {
	upstreamServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("X-Server-Version", "1.0")
		w.Header().Set("X-Internal", "internal-data")
		w.Header().Set("X-Public", "public-data")
		w.WriteHeader(http.StatusOK)
	}))
	defer upstreamServer.Close()

	hc := health.NewHealthChecker()
	defer hc.Close()

	r := router.NewRouter(hc)
	h := NewHandler(r, hc)

	r.AddRoute(config.RouteConfig{
		Match: config.MatchConfig{
			PathPrefix: "/",
		},
		Upstream:              upstreamServer.URL,
		RemoveResponseHeaders: []string{"X-Server-Version", "X-Internal"},
	})

	req := httptest.NewRequest("GET", "/", nil)
	w := httptest.NewRecorder()

	h.ServeProxy(w, req)

	if w.Header().Get("X-Server-Version") != "" {
		t.Fatalf("expected X-Server-Version to be removed")
	}
	if w.Header().Get("X-Internal") != "" {
		t.Fatalf("expected X-Internal to be removed")
	}
	if w.Header().Get("X-Public") == "" {
		t.Fatalf("expected X-Public to be present")
	}
}
