package router

import (
	"net/http"
	"net/url"
	"strings"
	"sync"

	"reverse-proxy-go/internal/config"
	"reverse-proxy-go/internal/health"
)

// Router handles routing requests to appropriate upstreams.
type Router struct {
	mu            sync.RWMutex
	routes        []config.RouteConfig
	healthChecker *health.HealthChecker
}

// NewRouter creates a new router.
func NewRouter(hc *health.HealthChecker) *Router {
	return &Router{
		routes:        []config.RouteConfig{},
		healthChecker: hc,
	}
}

// SetRoutes sets the routes for the router.
func (r *Router) SetRoutes(routes []config.RouteConfig) {
	r.mu.Lock()
	defer r.mu.Unlock()

	// Stop health checks for old routes
	oldUpstreams := make(map[string]bool)
	for _, route := range r.routes {
		oldUpstreams[route.Upstream] = true
	}

	// Identify new upstreams
	newUpstreams := make(map[string]bool)
	for _, route := range routes {
		if route.HealthCheck != nil {
			newUpstreams[route.Upstream] = true
		}
	}

	// Unregister upstreams no longer needed
	for upstream := range oldUpstreams {
		if !newUpstreams[upstream] {
			r.healthChecker.UnregisterUpstream(upstream)
		}
	}

	// Register new upstreams
	for _, route := range routes {
		if route.HealthCheck != nil {
			r.healthChecker.RegisterUpstream(
				route.Upstream,
				route.HealthCheck.Path,
				route.HealthCheck.IntervalMs,
			)
		}
	}

	r.routes = routes
}

// AddRoute adds a new route.
func (r *Router) AddRoute(route config.RouteConfig) {
	r.mu.Lock()
	defer r.mu.Unlock()

	if route.HealthCheck != nil {
		r.healthChecker.RegisterUpstream(
			route.Upstream,
			route.HealthCheck.Path,
			route.HealthCheck.IntervalMs,
		)
	}

	r.routes = append(r.routes, route)
}

// RemoveRoute removes a route by index.
func (r *Router) RemoveRoute(index int) {
	r.mu.Lock()
	defer r.mu.Unlock()

	if index < 0 || index >= len(r.routes) {
		return
	}

	removedRoute := r.routes[index]
	r.routes = append(r.routes[:index], r.routes[index+1:]...)

	// Check if upstream is still used by any other route
	stillUsed := false
	for _, route := range r.routes {
		if route.Upstream == removedRoute.Upstream {
			stillUsed = true
			break
		}
	}

	// Unregister if not used anymore
	if !stillUsed && removedRoute.HealthCheck != nil {
		r.healthChecker.UnregisterUpstream(removedRoute.Upstream)
	}
}

// GetRoutes returns a copy of the current routes.
func (r *Router) GetRoutes() []config.RouteConfig {
	r.mu.RLock()
	defer r.mu.RUnlock()

	routes := make([]config.RouteConfig, len(r.routes))
	copy(routes, r.routes)
	return routes
}

// MatchRoute finds the first route that matches the request.
// Returns nil if no route matches.
func (r *Router) MatchRoute(req *http.Request) *config.RouteConfig {
	r.mu.RLock()
	defer r.mu.RUnlock()

	for i := range r.routes {
		route := &r.routes[i]
		if r.matchesRoute(req, route) {
			return route
		}
	}

	return nil
}

// matchesRoute checks if a request matches a route's criteria.
func (r *Router) matchesRoute(req *http.Request, route *config.RouteConfig) bool {
	// If host is specified, it must match
	if route.Match.Host != "" && req.Host != route.Match.Host {
		return false
	}

	// If pathPrefix is specified, it must match
	if route.Match.PathPrefix != "" && !strings.HasPrefix(req.URL.Path, route.Match.PathPrefix) {
		return false
	}

	return true
}

// BuildDirector returns a Director function for httputil.ReverseProxy.
func (r *Router) BuildDirector(route *config.RouteConfig) func(*http.Request) {
	return func(req *http.Request) {
		// Parse upstream URL
		upstreamURL, err := url.Parse(route.Upstream)
		if err != nil {
			return
		}

		// Set the scheme and host from upstream
		req.URL.Scheme = upstreamURL.Scheme
		req.URL.Host = upstreamURL.Host

		// Strip prefix if configured
		if route.StripPrefix && route.Match.PathPrefix != "" {
			req.URL.Path = strings.TrimPrefix(req.URL.Path, route.Match.PathPrefix)
			// Ensure path starts with / or is empty
			if req.URL.Path == "" {
				req.URL.Path = "/"
			}
		}

		// Apply request header modifications
		if route.AddRequestHeaders != nil {
			for key, value := range route.AddRequestHeaders {
				req.Header.Set(key, value)
			}
		}

		if route.RemoveRequestHeaders != nil {
			for _, header := range route.RemoveRequestHeaders {
				req.Header.Del(header)
			}
		}

		// Strip hop-by-hop headers
		stripHopByHopHeaders(req.Header)

		// Set Host header to upstream host
		req.Host = upstreamURL.Host
		req.RequestURI = ""
	}
}

// BuildModifyResponse returns a ModifyResponse function for httputil.ReverseProxy.
func (r *Router) BuildModifyResponse(route *config.RouteConfig) func(*http.Response) error {
	return func(resp *http.Response) error {
		// Apply response header modifications
		if route.AddResponseHeaders != nil {
			for key, value := range route.AddResponseHeaders {
				resp.Header.Set(key, value)
			}
		}

		if route.RemoveResponseHeaders != nil {
			for _, header := range route.RemoveResponseHeaders {
				resp.Header.Del(header)
			}
		}

		return nil
	}
}

// stripHopByHopHeaders removes hop-by-hop headers from a request.
// These headers must not be forwarded to the upstream.
func stripHopByHopHeaders(h http.Header) {
	hopByHopHeaders := []string{
		"Connection",
		"Keep-Alive",
		"Proxy-Authenticate",
		"Proxy-Authorization",
		"TE",
		"Trailers",
		"Transfer-Encoding",
		"Upgrade",
	}

	for _, header := range hopByHopHeaders {
		h.Del(header)
	}
}
