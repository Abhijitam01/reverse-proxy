package health

import (
	"context"
	"net/http"
	"sync"
	"sync/atomic"
	"time"
)

// HealthChecker monitors the health of upstream servers.
type HealthChecker struct {
	mu               sync.RWMutex
	healthStatuses   map[string]*atomic.Bool
	cancelFunctions  map[string]context.CancelFunc
	client           *http.Client
}

// NewHealthChecker creates a new health checker.
func NewHealthChecker() *HealthChecker {
	return &HealthChecker{
		healthStatuses:  make(map[string]*atomic.Bool),
		cancelFunctions: make(map[string]context.CancelFunc),
		client: &http.Client{
			Timeout: 5 * time.Second,
		},
	}
}

// RegisterUpstream registers an upstream server for health checking.
// Returns immediately; health checks run in background.
func (hc *HealthChecker) RegisterUpstream(upstream, healthPath string, intervalMs int) {
	hc.mu.Lock()
	defer hc.mu.Unlock()

	// Skip if already registered
	if _, exists := hc.healthStatuses[upstream]; exists {
		return
	}

	// Initialize as healthy by default
	healthy := &atomic.Bool{}
	healthy.Store(true)
	hc.healthStatuses[upstream] = healthy

	// Create cancellation context
	ctx, cancel := context.WithCancel(context.Background())
	hc.cancelFunctions[upstream] = cancel

	// Start health check goroutine
	go hc.checkHealth(ctx, upstream, healthPath, time.Duration(intervalMs)*time.Millisecond)
}

// checkHealth periodically checks the health of an upstream server.
func (hc *HealthChecker) checkHealth(ctx context.Context, upstream, healthPath string, interval time.Duration) {
	ticker := time.NewTicker(interval)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			hc.performHealthCheck(upstream, healthPath)
		}
	}
}

// performHealthCheck performs a single health check request.
func (hc *HealthChecker) performHealthCheck(upstream, healthPath string) {
	healthURL := upstream + healthPath
	resp, err := hc.client.Get(healthURL)

	hc.mu.RLock()
	healthStatus, exists := hc.healthStatuses[upstream]
	hc.mu.RUnlock()

	// Upstream was unregistered, skip
	if !exists || healthStatus == nil {
		return
	}

	if err != nil {
		healthStatus.Store(false)
		return
	}
	defer resp.Body.Close()

	// Consider 2xx and 3xx as healthy
	isHealthy := resp.StatusCode >= 200 && resp.StatusCode < 400
	healthStatus.Store(isHealthy)
}

// IsHealthy returns whether an upstream is healthy.
func (hc *HealthChecker) IsHealthy(upstream string) bool {
	hc.mu.RLock()
	healthStatus, exists := hc.healthStatuses[upstream]
	hc.mu.RUnlock()

	if !exists {
		// If not registered, assume healthy
		return true
	}

	return healthStatus.Load()
}

// UnregisterUpstream stops health checking for an upstream.
func (hc *HealthChecker) UnregisterUpstream(upstream string) {
	hc.mu.Lock()
	defer hc.mu.Unlock()

	if cancel, exists := hc.cancelFunctions[upstream]; exists {
		cancel()
		delete(hc.cancelFunctions, upstream)
		delete(hc.healthStatuses, upstream)
	}
}

// GetStatus returns the health status of all registered upstreams.
func (hc *HealthChecker) GetStatus() map[string]bool {
	hc.mu.RLock()
	defer hc.mu.RUnlock()

	status := make(map[string]bool)
	for upstream, healthStatus := range hc.healthStatuses {
		status[upstream] = healthStatus.Load()
	}
	return status
}

// Close stops all health check goroutines.
func (hc *HealthChecker) Close() {
	hc.mu.Lock()
	defer hc.mu.Unlock()

	for _, cancel := range hc.cancelFunctions {
		cancel()
	}
	hc.cancelFunctions = make(map[string]context.CancelFunc)
	hc.healthStatuses = make(map[string]*atomic.Bool)
}
