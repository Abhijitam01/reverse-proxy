# Reverse Proxy (Go)

A high-performance HTTP reverse proxy written in Go with host/path routing, header manipulation, and health checking.

## Features

- **Host & Path Routing**: Route requests based on host and/or path prefix
- **Prefix Stripping**: Automatically remove matched path prefixes before forwarding
- **Header Manipulation**: Add/remove request and response headers
- **Upstream Health Checking**: Background health monitoring with atomic status tracking
- **Management API**: Dynamically add/remove/list routes without restart
- **Thread-Safe**: All operations protected with RWMutex for safe concurrent access
- **Standard Library**: Uses `net/http/httputil.ReverseProxy` for proven forwarding engine

## Architecture

```
cmd/server/main.go          - Entry point, HTTP server setup
internal/config/config.go   - Configuration loading and validation
internal/router/router.go   - Route matching and ReverseProxy director
internal/health/health.go   - Upstream health monitoring
internal/handler/handler.go - HTTP request/response handlers
```

## Configuration

Configuration is JSON-based. See `config.example.json` for a complete example.

### Config Structure

```json
{
  "listenAddr": ":8080",
  "routes": [
    {
      "match": {
        "host": "api.example.com",
        "pathPrefix": "/users"
      },
      "upstream": "http://api-backend:3000",
      "stripPrefix": true,
      "addRequestHeaders": {
        "X-Forwarded-By": "proxy"
      },
      "removeRequestHeaders": ["X-Internal"],
      "addResponseHeaders": {
        "X-Cache": "miss"
      },
      "removeResponseHeaders": ["Server"],
      "healthCheck": {
        "path": "/health",
        "intervalMs": 10000
      }
    }
  ]
}
```

### Field Descriptions

- `listenAddr`: Address to listen on (default: `:8080`)
- `routes`: Array of route configurations
  - `match.host`: Match requests with this host (optional)
  - `match.pathPrefix`: Match requests with this path prefix (at least one required)
  - `upstream`: Target upstream URL (required)
  - `stripPrefix`: Remove matched `pathPrefix` before forwarding
  - `addRequestHeaders`: Headers to add to requests (optional)
  - `removeRequestHeaders`: Headers to remove from requests (optional)
  - `addResponseHeaders`: Headers to add to responses (optional)
  - `removeResponseHeaders`: Headers to remove from responses (optional)
  - `healthCheck`: Health check configuration (optional)
    - `path`: Health check endpoint (e.g., `/health`)
    - `intervalMs`: Check interval in milliseconds

### Routing Rules

Routes are matched in order. The first matching route is used.

A request matches a route if:
- If `match.host` is specified, the request host must match exactly
- If `match.pathPrefix` is specified, the request path must start with this prefix
- At least one of `host` or `pathPrefix` must match (AND logic)

### Health Checking

When a health check is configured:
- Background goroutine monitors the upstream at the configured interval
- Health status is stored atomically (thread-safe reads with no locks)
- Unhealthy upstreams return 502 Bad Gateway
- Health status codes 2xx-3xx are considered healthy

## Building

```bash
go build -o bin/reverse-proxy-go ./cmd/server
```

## Running

```bash
# With default config.json
./bin/reverse-proxy-go

# With custom config
./bin/reverse-proxy-go -config /path/to/config.json
```

## Management API

### List Routes

```bash
curl http://localhost:8080/_proxy/routes
```

Response:
```json
{
  "routes": [
    {
      "index": 0,
      "match": { "pathPrefix": "/api" },
      "upstream": "http://localhost:3000",
      "stripPrefix": true,
      "isHealthy": true,
      ...
    }
  ]
}
```

### Add Route

```bash
curl -X POST http://localhost:8080/_proxy/routes \
  -H "Content-Type: application/json" \
  -d '{
    "match": { "pathPrefix": "/users" },
    "upstream": "http://users-service:3000",
    "stripPrefix": true
  }'
```

### Remove Route

```bash
curl -X DELETE http://localhost:8080/_proxy/routes/0
```

### Health Status

```bash
curl http://localhost:8080/_proxy/health
```

Response:
```json
{
  "status": "healthy",
  "routes": {
    "http://localhost:3000": true,
    "http://localhost:4000": false
  }
}
```

## Testing

```bash
go test ./...
```

All tests use `httptest.NewServer` for upstream mocks and cover:
- Basic request forwarding
- Prefix stripping
- Host matching
- Header manipulation (add/remove)
- Route ordering
- Health checking
- 404/502 error cases
- Request/response body passthrough

## Implementation Details

### Thread Safety

- Route list protected by `sync.RWMutex` in Router
- Health status stored in `sync.atomic.Bool` for lock-free reads
- Cancel functions map protected by RWMutex in HealthChecker

### Goroutine Lifecycle

Health check goroutines use `context.Context` for clean cancellation:
- Created when route with health check is added
- Stopped when route is removed or proxy shutdowns
- Ticker cleanup ensures no resource leaks

### ReverseProxy Integration

Uses `httputil.ReverseProxy` with:
- Custom `Director`: Sets upstream URL, applies header modifications, strips prefix
- Custom `ModifyResponse`: Adds/removes response headers
- Custom `ErrorHandler`: Returns 502 on proxy errors

Hop-by-hop headers are automatically stripped to prevent forwarding issues.

## Example: Multi-Service Setup

```json
{
  "listenAddr": ":8080",
  "routes": [
    {
      "match": { "pathPrefix": "/api/users" },
      "upstream": "http://users-service:3000",
      "stripPrefix": true,
      "addRequestHeaders": { "X-Service": "users" },
      "healthCheck": { "path": "/health", "intervalMs": 5000 }
    },
    {
      "match": { "pathPrefix": "/api/orders" },
      "upstream": "http://orders-service:3001",
      "stripPrefix": true,
      "addRequestHeaders": { "X-Service": "orders" },
      "healthCheck": { "path": "/health", "intervalMs": 5000 }
    },
    {
      "match": { "host": "admin.example.com" },
      "upstream": "http://admin-dashboard:3000",
      "stripPrefix": false,
      "addResponseHeaders": { "X-Protected": "true" }
    }
  ]
}
```
