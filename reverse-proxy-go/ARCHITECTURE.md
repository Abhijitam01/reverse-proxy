# Reverse Proxy Architecture

## Overview

This is a high-performance HTTP reverse proxy built with Go's standard library. It provides intelligent request routing, header manipulation, and health monitoring with a thread-safe, lock-free design where possible.

## Core Design Principles

### 1. Thread Safety Without Contention
- **Route matching**: Protected by RWMutex for concurrent reads, minimal write contention
- **Health status**: Atomic booleans for lock-free reads during proxying (hot path)
- **Health check goroutines**: Managed with context cancellation for clean shutdown

### 2. Separation of Concerns
- **Config**: Loading, parsing, validation
- **Router**: Route matching and httputil.ReverseProxy director/modifier functions
- **Health**: Background monitoring with atomic status tracking
- **Handler**: HTTP endpoint handlers (proxy + management API)

### 3. Immutability
- Routes are matched by value, not reference
- Director and ModifyResponse functions are pure (no state modification)
- Atomic booleans ensure status reads never block writers

## Architecture Diagram

```
┌─────────────────────────────────────────────────────────────┐
│                    HTTP Request                             │
└────────────────────────┬────────────────────────────────────┘
                         │
                         ▼
            ┌────────────────────────┐
            │  Chi Router Handler    │
            │  (routes to handlers)  │
            └────────┬───────────────┘
                     │
         ┌───────────┴───────────┐
         │                       │
         ▼                       ▼
    ┌─────────────┐      ┌──────────────────┐
    │Proxy Handler│      │Management Handler│
    └──────┬──────┘      └──────────────────┘
           │              /_proxy/routes
           │              /_proxy/health
           │
           ▼
    ┌──────────────────┐
    │ Router.MatchRoute│ (RWMutex-protected reads)
    └────────┬─────────┘
             │
             ├─► No match → 404
             │
             ├─► Route found
             │   └─► HealthChecker.IsHealthy?
             │       ├─► No → 502
             │       └─► Yes → ReverseProxy
             │
             ▼
    ┌──────────────────────────────┐
    │ httputil.ReverseProxy        │
    ├──────────────────────────────┤
    │ - Director:                  │
    │   • Set upstream URL         │
    │   • Strip prefix if needed   │
    │   • Add/remove headers       │
    │                              │
    │ - ModifyResponse:            │
    │   • Add/remove headers       │
    │                              │
    │ - ErrorHandler:              │
    │   • Return 502 on error      │
    └──────────────────────────────┘
           │
           ▼
      [Upstream]
```

## Component Details

### 1. Config (`internal/config/config.go`)

**Responsibility**: Configuration management

**Key Types**:
- `RouteConfig`: Single route definition
- `ProxyConfig`: Root configuration with listen address and routes

**Key Functions**:
- `LoadConfig(path)`: Parse JSON file, validate structure
- `DefaultConfig()`: Generate minimal test configuration

**Validation**:
- Routes must have upstream URL
- Routes must have at least host OR pathPrefix match
- At least one route must be configured

### 2. Router (`internal/router/router.go`)

**Responsibility**: Route matching and ReverseProxy setup

**Key Types**:
- `Router`: Thread-safe route store and matcher

**Key Methods**:
- `SetRoutes()`: Replace all routes (used at startup)
- `AddRoute()`: Add route with health check registration
- `RemoveRoute()`: Remove route and unregister health checks
- `MatchRoute()`: Find first matching route (O(n) but typically small n)
- `BuildDirector()`: Create Director function for httputil.ReverseProxy
- `BuildModifyResponse()`: Create response modifier function

**Thread Safety**:
```go
r.mu.RWMutex     // Protects routes slice
r.routes         // Routes matched in order
```

**Routing Algorithm**:
```
for each route in order:
  if (host is specified AND matches) OR (pathPrefix is specified AND matches):
    return route
return nil
```

**Director Function** (sets on request):
1. Parse upstream URL
2. Set request scheme/host from upstream
3. Strip matched pathPrefix if configured
4. Add/remove request headers
5. Strip hop-by-hop headers
6. Set request Host header

**ModifyResponse Function** (modifies response):
1. Add response headers if configured
2. Remove response headers if configured
3. Return nil (success)

### 3. Health Checker (`internal/health/health.go`)

**Responsibility**: Background upstream monitoring

**Key Types**:
- `HealthChecker`: Manages health check goroutines and status

**Key Methods**:
- `RegisterUpstream()`: Start health checks for an upstream
- `IsHealthy()`: Get current health status (lock-free read)
- `UnregisterUpstream()`: Stop health checks for an upstream
- `GetStatus()`: Get all upstream statuses
- `Close()`: Stop all health checks

**Thread Safety**:
```go
hc.mu.RWMutex                          // Protects maps below
hc.healthStatuses map[string]*atomic.Bool  // Lock-free reads
hc.cancelFunctions map[string]CancelFunc   // Graceful shutdown
```

**Health Check Loop**:
```
1. Start at interval T
2. GET {upstream}{healthPath}
3. Read atomic status (no lock needed)
4. Store result (2xx-3xx = healthy, otherwise = unhealthy)
5. Repeat
```

**Goroutine Lifecycle**:
- One goroutine per upstream with health checks
- Uses context.Context for cancellation
- Ticker ensures periodic cleanup
- Safe to call IsHealthy() while checks run (lock-free)

**Error Handling**:
- Network errors → unhealthy
- Missing upstream → treat as healthy (safety default)
- Concurrent unregister → safely ignore stale updates

### 4. Handler (`internal/handler/handler.go`)

**Responsibility**: HTTP endpoint handling

**Key Methods**:

**Proxy Endpoints**:
- `ServeProxy()`: Main reverse proxy handler
  - Matches route
  - Checks health
  - Forwards via httputil.ReverseProxy

**Management Endpoints**:
- `ListRoutes()`: GET /_proxy/routes
- `AddRoute()`: POST /_proxy/routes  
- `RemoveRoute()`: DELETE /_proxy/routes/{index}
- `Health()`: GET /_proxy/health

**Request/Response Flow**:
```
Request → ServeProxy()
   └─→ MatchRoute() (RWMutex read)
       ├─→ No match → 404
       └─→ IsHealthy() (atomic read, no lock)
           ├─→ Unhealthy → 502
           └─→ Healthy → ReverseProxy.ServeHTTP()
               └─→ Director() → upstream request
               └─→ ModifyResponse() → response headers
```

## Data Structures

### Routes Storage
```go
type Router struct {
    mu     sync.RWMutex
    routes []config.RouteConfig
}
```

Routes are stored in a slice, not a map, to preserve order. Order matters for routing priority.

### Health Status Storage
```go
type HealthChecker struct {
    mu              sync.RWMutex
    healthStatuses  map[string]*atomic.Bool    // Lock-free reads
    cancelFunctions map[string]context.CancelFunc
}
```

Health status uses `atomic.Bool` so proxying hot path requires no locks.

## Concurrency Model

### Request Proxying (Hot Path)
```
No locks needed:
- Router.MatchRoute() → RWMutex.RLock()  (brief, uncontended)
- HealthChecker.IsHealthy() → atomic.Load() (no locks)
- httputil.ReverseProxy → handles concurrency safely
```

### Route Modification (Cold Path)
```
Protected by locks:
- Router.AddRoute() → RWMutex.Lock()
- Router.RemoveRoute() → RWMutex.Lock()
- Management API handlers → fully serialized
```

### Health Checks (Background)
```
Isolated goroutines:
- One per upstream with health check
- Only access own atomic.Bool for writes
- Minimal contention (atomic operations)
- Context-based cancellation for shutdown
```

## Performance Characteristics

### Route Matching: O(n)
- Linear search through routes
- Typically 10-50 routes in practice
- Could be optimized with Radix tree if needed

### Health Check: O(1)
- Atomic read for status
- No locks on proxying hot path
- Background goroutines isolated

### Memory: O(n + h)
- n = number of routes
- h = number of upstreams with health checks
- Small constant factors

## Error Handling Strategy

### Client Errors
- **400**: Invalid management API request
- **404**: Route not matched
- **502**: Upstream unhealthy or unreachable
- **Other**: Passed through from upstream

### Upstream Errors
- Network errors → marked unhealthy
- Non-200 status → passed through (unless health check)
- Timeout (5s) → treated as unhealthy

### Configuration Errors
- File not found → fatal
- Invalid JSON → fatal
- Missing required fields → fatal

## Security Considerations

### Headers
- Hop-by-hop headers stripped automatically
- User can remove sensitive headers via config
- Forward headers not set automatically (request Host is upstream host)

### Health Checks
- Uses standard HTTP client with 5s timeout
- Respects 2xx-3xx as healthy (broad tolerance)
- Network errors safely mark unhealthy

### No Built-in Auth
- Management API unprotected (deploy behind auth proxy)
- All routes equally accessible
- Client can add auth via header manipulation

## Testing Strategy

### Unit Tests
- 40+ tests covering core logic
- 100% coverage: config, health
- 82.9% coverage: router
- 63.1% coverage: handler

### Test Patterns
- httptest.NewServer for upstream mocks
- Table-driven tests for variants
- Concurrent tests for race conditions
- Health check timing tests for interval verification

## Extension Points

### Adding Features
1. **Custom director logic**: Modify `BuildDirector()` method
2. **Custom response logic**: Modify `BuildModifyResponse()` method
3. **Alternative matching**: Change `matchesRoute()` logic
4. **Different health metrics**: Add fields to HealthCheckConfig
5. **New management endpoints**: Add handler methods

### Performance Tuning
1. **Health check frequency**: Adjust intervalMs per route
2. **Timeout values**: Edit HealthChecker client timeout
3. **Route order**: Match frequently-hit routes first
4. **Health status fallback**: Change IsHealthy() default

## Dependencies

- **Standard Library**: net, http, net/http/httputil (core)
- **Chi Router**: For management API HTTP routing
- **No external dependencies for core proxying**

This minimal dependency set ensures:
- Fast builds
- Small binary (9.2 MB uncompressed)
- Easy deployment
- Few security updates needed
