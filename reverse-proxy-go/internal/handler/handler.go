package handler

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httputil"
	"strconv"
	"strings"

	"github.com/go-chi/chi/v5"

	"reverse-proxy-go/internal/config"
	"reverse-proxy-go/internal/health"
	"reverse-proxy-go/internal/router"
)

type Handler struct {
	router        *router.Router
	healthChecker *health.HealthChecker
}

func NewHandler(r *router.Router, hc *health.HealthChecker) *Handler {
	return &Handler{
		router:        r,
		healthChecker: hc,
	}
}

func (h *Handler) ServeProxy(w http.ResponseWriter, r *http.Request) {
	route := h.router.MatchRoute(r)
	if route == nil {
		http.Error(w, "Not Found", http.StatusNotFound)
		return
	}

	if !h.healthChecker.IsHealthy(route.Upstream) {
		http.Error(w, "Bad Gateway", http.StatusBadGateway)
		return
	}

	proxy := &httputil.ReverseProxy{
		Director:       h.router.BuildDirector(route),
		ModifyResponse: h.router.BuildModifyResponse(route),
		ErrorHandler: func(w http.ResponseWriter, r *http.Request, err error) {
			http.Error(w, "Bad Gateway", http.StatusBadGateway)
		},
	}

	proxy.ServeHTTP(w, r)
}

type ListRoutesResponse struct {
	Routes []RouteInfo `json:"routes"`
}

type RouteInfo struct {
	Index                 int                       `json:"index"`
	Match                 config.MatchConfig        `json:"match"`
	Upstream              string                    `json:"upstream"`
	StripPrefix           bool                      `json:"stripPrefix"`
	AddRequestHeaders     map[string]string         `json:"addRequestHeaders"`
	RemoveRequestHeaders  []string                  `json:"removeRequestHeaders"`
	AddResponseHeaders    map[string]string         `json:"addResponseHeaders"`
	RemoveResponseHeaders []string                  `json:"removeResponseHeaders"`
	HealthCheck           *config.HealthCheckConfig `json:"healthCheck"`
	IsHealthy             bool                      `json:"isHealthy"`
}

func (h *Handler) ListRoutes(w http.ResponseWriter, r *http.Request) {
	routes := h.router.GetRoutes()
	healthStatus := h.healthChecker.GetStatus()

	routeInfos := make([]RouteInfo, len(routes))
	for i, route := range routes {
		routeInfos[i] = RouteInfo{
			Index:                 i,
			Match:                 route.Match,
			Upstream:              route.Upstream,
			StripPrefix:           route.StripPrefix,
			AddRequestHeaders:     route.AddRequestHeaders,
			RemoveRequestHeaders:  route.RemoveRequestHeaders,
			AddResponseHeaders:    route.AddResponseHeaders,
			RemoveResponseHeaders: route.RemoveResponseHeaders,
			HealthCheck:           route.HealthCheck,
			IsHealthy:             healthStatus[route.Upstream],
		}
	}

	response := ListRoutesResponse{Routes: routeInfos}
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(response)
}

type AddRouteRequest struct {
	Match                 config.MatchConfig        `json:"match"`
	Upstream              string                    `json:"upstream"`
	StripPrefix           bool                      `json:"stripPrefix"`
	AddRequestHeaders     map[string]string         `json:"addRequestHeaders"`
	RemoveRequestHeaders  []string                  `json:"removeRequestHeaders"`
	AddResponseHeaders    map[string]string         `json:"addResponseHeaders"`
	RemoveResponseHeaders []string                  `json:"removeResponseHeaders"`
	HealthCheck           *config.HealthCheckConfig `json:"healthCheck"`
}

func (h *Handler) AddRoute(w http.ResponseWriter, r *http.Request) {
	var req AddRouteRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, fmt.Sprintf("Invalid request body: %v", err), http.StatusBadRequest)
		return
	}

	if req.Upstream == "" {
		http.Error(w, "upstream is required", http.StatusBadRequest)
		return
	}

	if req.Match.Host == "" && req.Match.PathPrefix == "" {
		http.Error(w, "at least one of host or pathPrefix must be specified", http.StatusBadRequest)
		return
	}

	route := config.RouteConfig{
		Match:                 req.Match,
		Upstream:              req.Upstream,
		StripPrefix:           req.StripPrefix,
		AddRequestHeaders:     req.AddRequestHeaders,
		RemoveRequestHeaders:  req.RemoveRequestHeaders,
		AddResponseHeaders:    req.AddResponseHeaders,
		RemoveResponseHeaders: req.RemoveResponseHeaders,
		HealthCheck:           req.HealthCheck,
	}

	h.router.AddRoute(route)

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusCreated)
	json.NewEncoder(w).Encode(route)
}

func (h *Handler) RemoveRoute(w http.ResponseWriter, r *http.Request) {
	indexStr := chi.URLParam(r, "index")
	index, err := strconv.Atoi(indexStr)
	if err != nil {
		http.Error(w, "Invalid index", http.StatusBadRequest)
		return
	}

	routes := h.router.GetRoutes()
	if index < 0 || index >= len(routes) {
		http.Error(w, "Route not found", http.StatusNotFound)
		return
	}

	h.router.RemoveRoute(index)
	w.WriteHeader(http.StatusNoContent)
}

type HealthResponse struct {
	Status string          `json:"status"`
	Routes map[string]bool `json:"routes"`
}

func (h *Handler) Health(w http.ResponseWriter, r *http.Request) {
	healthStatus := h.healthChecker.GetStatus()
	routes := h.router.GetRoutes()

	overallHealthy := len(routes) > 0 && len(healthStatus) > 0
	if overallHealthy {
		for _, healthy := range healthStatus {
			if healthy {
				overallHealthy = true
				break
			}
		}
	}

	status := "unhealthy"
	if overallHealthy || len(routes) == 0 {
		status = "healthy"
	}

	response := HealthResponse{
		Status: status,
		Routes: healthStatus,
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(response)
}

func IsManagementPath(path string) bool {
	return strings.HasPrefix(path, "/_proxy/")
}

func ReadBody(r *http.Request) (string, error) {
	body, err := io.ReadAll(r.Body)
	if err != nil {
		return "", err
	}
	r.Body = io.NopCloser(strings.NewReader(string(body)))
	return string(body), nil
}
