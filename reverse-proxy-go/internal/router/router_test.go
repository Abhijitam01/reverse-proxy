package router

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"reverse-proxy-go/internal/config"
	"reverse-proxy-go/internal/health"
)

// TestRouteMatching tests route matching logic.
func TestRouteMatching(t *testing.T) {
	hc := health.NewHealthChecker()
	defer hc.Close()

	r := NewRouter(hc)

	r.AddRoute(config.RouteConfig{
		Match: config.MatchConfig{
			PathPrefix: "/api",
		},
		Upstream: "http://localhost:3000",
	})

	req := httptest.NewRequest("GET", "/api/users", nil)
	matched := r.MatchRoute(req)

	if matched == nil {
		t.Fatalf("expected route to match")
	}
	if matched.Upstream != "http://localhost:3000" {
		t.Fatalf("wrong route matched")
	}
}

// TestRouteMatchingHost tests host-based route matching.
func TestRouteMatchingHost(t *testing.T) {
	hc := health.NewHealthChecker()
	defer hc.Close()

	r := NewRouter(hc)

	r.AddRoute(config.RouteConfig{
		Match: config.MatchConfig{
			Host: "api.example.com",
		},
		Upstream: "http://api-backend:3000",
	})

	req := httptest.NewRequest("GET", "http://api.example.com/users", nil)
	req.Host = "api.example.com"
	matched := r.MatchRoute(req)

	if matched == nil {
		t.Fatalf("expected route to match host")
	}
}

// TestRouteNoMatch tests when no route matches.
func TestRouteNoMatch(t *testing.T) {
	hc := health.NewHealthChecker()
	defer hc.Close()

	r := NewRouter(hc)

	r.AddRoute(config.RouteConfig{
		Match: config.MatchConfig{
			PathPrefix: "/api",
		},
		Upstream: "http://localhost:3000",
	})

	req := httptest.NewRequest("GET", "/other", nil)
	matched := r.MatchRoute(req)

	if matched != nil {
		t.Fatalf("expected no route to match")
	}
}

// TestAddRoute tests adding routes.
func TestAddRoute(t *testing.T) {
	hc := health.NewHealthChecker()
	defer hc.Close()

	r := NewRouter(hc)

	r.AddRoute(config.RouteConfig{
		Upstream: "http://localhost:3000",
		Match:    config.MatchConfig{PathPrefix: "/api"},
	})

	routes := r.GetRoutes()
	if len(routes) != 1 {
		t.Fatalf("expected 1 route, got %d", len(routes))
	}
}

// TestRemoveRoute tests removing routes.
func TestRemoveRoute(t *testing.T) {
	hc := health.NewHealthChecker()
	defer hc.Close()

	r := NewRouter(hc)

	r.AddRoute(config.RouteConfig{
		Upstream: "http://localhost:3000",
		Match:    config.MatchConfig{PathPrefix: "/api"},
	})

	r.RemoveRoute(0)

	routes := r.GetRoutes()
	if len(routes) != 0 {
		t.Fatalf("expected 0 routes after removal")
	}
}

// TestPrefixStripping tests Director with prefix stripping.
func TestPrefixStripping(t *testing.T) {
	hc := health.NewHealthChecker()
	defer hc.Close()

	r := NewRouter(hc)

	route := config.RouteConfig{
		Upstream:    "http://localhost:3000",
		StripPrefix: true,
		Match:       config.MatchConfig{PathPrefix: "/api"},
	}

	director := r.BuildDirector(&route)

	req := httptest.NewRequest("GET", "http://localhost:8080/api/users", nil)
	director(req)

	if req.URL.Path != "/users" {
		t.Fatalf("expected path '/users', got %q", req.URL.Path)
	}
}

// TestPrefixStrippingWithoutStripPrefix tests Director without stripping.
func TestPrefixStrippingWithoutStripPrefix(t *testing.T) {
	hc := health.NewHealthChecker()
	defer hc.Close()

	r := NewRouter(hc)

	route := config.RouteConfig{
		Upstream:    "http://localhost:3000",
		StripPrefix: false,
		Match:       config.MatchConfig{PathPrefix: "/api"},
	}

	director := r.BuildDirector(&route)

	req := httptest.NewRequest("GET", "http://localhost:8080/api/users", nil)
	director(req)

	if req.URL.Path != "/api/users" {
		t.Fatalf("expected path '/api/users', got %q", req.URL.Path)
	}
}

// TestDirectorSetsUpstreamURL tests that Director sets upstream URL.
func TestDirectorSetsUpstreamURL(t *testing.T) {
	hc := health.NewHealthChecker()
	defer hc.Close()

	r := NewRouter(hc)

	route := config.RouteConfig{
		Upstream: "http://api-backend:3000",
		Match:    config.MatchConfig{PathPrefix: "/"},
	}

	director := r.BuildDirector(&route)

	req := httptest.NewRequest("GET", "/test", nil)
	director(req)

	if req.URL.Host != "api-backend:3000" {
		t.Fatalf("expected host 'api-backend:3000', got %q", req.URL.Host)
	}
	if req.URL.Scheme != "http" {
		t.Fatalf("expected scheme 'http', got %q", req.URL.Scheme)
	}
}

// TestDirectorAddRequestHeaders tests adding request headers.
func TestDirectorAddRequestHeaders(t *testing.T) {
	hc := health.NewHealthChecker()
	defer hc.Close()

	r := NewRouter(hc)

	route := config.RouteConfig{
		Upstream: "http://localhost:3000",
		Match:    config.MatchConfig{PathPrefix: "/"},
		AddRequestHeaders: map[string]string{
			"X-Custom": "value",
		},
	}

	director := r.BuildDirector(&route)

	req := httptest.NewRequest("GET", "/", nil)
	director(req)

	if req.Header.Get("X-Custom") != "value" {
		t.Fatalf("expected X-Custom header to be set")
	}
}

// TestDirectorRemoveRequestHeaders tests removing request headers.
func TestDirectorRemoveRequestHeaders(t *testing.T) {
	hc := health.NewHealthChecker()
	defer hc.Close()

	r := NewRouter(hc)

	route := config.RouteConfig{
		Upstream: "http://localhost:3000",
		Match:    config.MatchConfig{PathPrefix: "/"},
		RemoveRequestHeaders: []string{"X-Remove"},
	}

	director := r.BuildDirector(&route)

	req := httptest.NewRequest("GET", "/", nil)
	req.Header.Set("X-Remove", "value")
	director(req)

	if req.Header.Get("X-Remove") != "" {
		t.Fatalf("expected X-Remove header to be removed")
	}
}

// TestModifyResponseAddHeaders tests adding response headers.
func TestModifyResponseAddHeaders(t *testing.T) {
	hc := health.NewHealthChecker()
	defer hc.Close()

	r := NewRouter(hc)

	route := config.RouteConfig{
		Upstream: "http://localhost:3000",
		Match:    config.MatchConfig{PathPrefix: "/"},
		AddResponseHeaders: map[string]string{
			"X-Added": "value",
		},
	}

	modifier := r.BuildModifyResponse(&route)

	resp := &http.Response{
		Header: make(http.Header),
	}

	modifier(resp)

	if resp.Header.Get("X-Added") != "value" {
		t.Fatalf("expected X-Added header to be set in response")
	}
}

// TestModifyResponseRemoveHeaders tests removing response headers.
func TestModifyResponseRemoveHeaders(t *testing.T) {
	hc := health.NewHealthChecker()
	defer hc.Close()

	r := NewRouter(hc)

	route := config.RouteConfig{
		Upstream: "http://localhost:3000",
		Match:    config.MatchConfig{PathPrefix: "/"},
		RemoveResponseHeaders: []string{"X-Remove"},
	}

	modifier := r.BuildModifyResponse(&route)

	resp := &http.Response{
		Header: make(http.Header),
	}
	resp.Header.Set("X-Remove", "value")

	modifier(resp)

	if resp.Header.Get("X-Remove") != "" {
		t.Fatalf("expected X-Remove header to be removed from response")
	}
}

// TestSetRoutes tests setting multiple routes.
func TestSetRoutes(t *testing.T) {
	hc := health.NewHealthChecker()
	defer hc.Close()

	r := NewRouter(hc)

	routes := []config.RouteConfig{
		{
			Upstream: "http://localhost:3000",
			Match:    config.MatchConfig{PathPrefix: "/api"},
		},
		{
			Upstream: "http://localhost:4000",
			Match:    config.MatchConfig{PathPrefix: "/web"},
		},
	}

	r.SetRoutes(routes)

	stored := r.GetRoutes()
	if len(stored) != 2 {
		t.Fatalf("expected 2 routes, got %d", len(stored))
	}
}
