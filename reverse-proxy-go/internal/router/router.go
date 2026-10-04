package router

import (
	"net/http"
	"net/url"
	"strings"
	"sync"

	"reverse-proxy-go/internal/config"
	"reverse-proxy-go/internal/health"
)

type Router struct {
	mu            sync.RWMutex
	routes        []config.RouteConfig
	healthChecker *health.HealthChecker
}

func NewRouter(hc *health.HealthChecker) *Router {
	return &Router{
		routes:        []config.RouteConfig{},
		healthChecker: hc,
	}
}

func (r *Router) SetRoutes(routes []config.RouteConfig) {
	r.mu.Lock()
	defer r.mu.Unlock()

	oldUpstreams := make(map[string]bool)
	for _, route := range r.routes {
		oldUpstreams[route.Upstream] = true
	}

	newUpstreams := make(map[string]bool)
	for _, route := range routes {
		if route.HealthCheck != nil {
			newUpstreams[route.Upstream] = true
		}
	}

	for upstream := range oldUpstreams {
		if !newUpstreams[upstream] {
			r.healthChecker.UnregisterUpstream(upstream)
		}
	}

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

func (r *Router) RemoveRoute(index int) {
	r.mu.Lock()
	defer r.mu.Unlock()

	if index < 0 || index >= len(r.routes) {
		return
	}

	removedRoute := r.routes[index]
	r.routes = append(r.routes[:index], r.routes[index+1:]...)

	stillUsed := false
	for _, route := range r.routes {
		if route.Upstream == removedRoute.Upstream {
			stillUsed = true
			break
		}
	}

	if !stillUsed && removedRoute.HealthCheck != nil {
		r.healthChecker.UnregisterUpstream(removedRoute.Upstream)
	}
}

func (r *Router) GetRoutes() []config.RouteConfig {
	r.mu.RLock()
	defer r.mu.RUnlock()

	routes := make([]config.RouteConfig, len(r.routes))
	copy(routes, r.routes)
	return routes
}

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

func (r *Router) matchesRoute(req *http.Request, route *config.RouteConfig) bool {
	if route.Match.Host != "" && req.Host != route.Match.Host {
		return false
	}

	if route.Match.PathPrefix != "" && !strings.HasPrefix(req.URL.Path, route.Match.PathPrefix) {
		return false
	}

	return true
}

func (r *Router) BuildDirector(route *config.RouteConfig) func(*http.Request) {
	return func(req *http.Request) {
		upstreamURL, err := url.Parse(route.Upstream)
		if err != nil {
			return
		}

		req.URL.Scheme = upstreamURL.Scheme
		req.URL.Host = upstreamURL.Host

		if route.StripPrefix && route.Match.PathPrefix != "" {
			req.URL.Path = strings.TrimPrefix(req.URL.Path, route.Match.PathPrefix)
			if req.URL.Path == "" {
				req.URL.Path = "/"
			}
		}

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

		stripHopByHopHeaders(req.Header)

		req.Host = upstreamURL.Host
		req.RequestURI = ""
	}
}

func (r *Router) BuildModifyResponse(route *config.RouteConfig) func(*http.Response) error {
	return func(resp *http.Response) error {
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
