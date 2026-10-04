package health

import (
	"context"
	"net/http"
	"sync"
	"sync/atomic"
	"time"
)

type HealthChecker struct {
	mu              sync.RWMutex
	healthStatuses  map[string]*atomic.Bool
	cancelFunctions map[string]context.CancelFunc
	client          *http.Client
}

func NewHealthChecker() *HealthChecker {
	return &HealthChecker{
		healthStatuses:  make(map[string]*atomic.Bool),
		cancelFunctions: make(map[string]context.CancelFunc),
		client: &http.Client{
			Timeout: 5 * time.Second,
		},
	}
}

func (hc *HealthChecker) RegisterUpstream(upstream, healthPath string, intervalMs int) {
	hc.mu.Lock()
	defer hc.mu.Unlock()

	if _, exists := hc.healthStatuses[upstream]; exists {
		return
	}

	healthy := &atomic.Bool{}
	healthy.Store(true)
	hc.healthStatuses[upstream] = healthy

	ctx, cancel := context.WithCancel(context.Background())
	hc.cancelFunctions[upstream] = cancel

	go hc.checkHealth(ctx, upstream, healthPath, time.Duration(intervalMs)*time.Millisecond)
}

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

func (hc *HealthChecker) performHealthCheck(upstream, healthPath string) {
	healthURL := upstream + healthPath
	resp, err := hc.client.Get(healthURL)

	hc.mu.RLock()
	healthStatus, exists := hc.healthStatuses[upstream]
	hc.mu.RUnlock()

	if !exists || healthStatus == nil {
		return
	}

	if err != nil {
		healthStatus.Store(false)
		return
	}
	defer resp.Body.Close()

	isHealthy := resp.StatusCode >= 200 && resp.StatusCode < 400
	healthStatus.Store(isHealthy)
}

func (hc *HealthChecker) IsHealthy(upstream string) bool {
	hc.mu.RLock()
	healthStatus, exists := hc.healthStatuses[upstream]
	hc.mu.RUnlock()

	if !exists {
		return true
	}

	return healthStatus.Load()
}

func (hc *HealthChecker) UnregisterUpstream(upstream string) {
	hc.mu.Lock()
	defer hc.mu.Unlock()

	if cancel, exists := hc.cancelFunctions[upstream]; exists {
		cancel()
		delete(hc.cancelFunctions, upstream)
		delete(hc.healthStatuses, upstream)
	}
}

func (hc *HealthChecker) GetStatus() map[string]bool {
	hc.mu.RLock()
	defer hc.mu.RUnlock()

	status := make(map[string]bool)
	for upstream, healthStatus := range hc.healthStatuses {
		status[upstream] = healthStatus.Load()
	}
	return status
}

func (hc *HealthChecker) Close() {
	hc.mu.Lock()
	defer hc.mu.Unlock()

	for _, cancel := range hc.cancelFunctions {
		cancel()
	}
	hc.cancelFunctions = make(map[string]context.CancelFunc)
	hc.healthStatuses = make(map[string]*atomic.Bool)
}
