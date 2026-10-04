# Quick Start Guide

## Build

```bash
cd /Users/abhijitam/Developer/project-all/reverse-proxy/reverse-proxy-go
make build
```

Binary created at `bin/reverse-proxy-go`

## Run with Example Config

```bash
./bin/reverse-proxy-go -config config.example.json
```

This starts the proxy on `:8080` with three example routes:
- `/api/*` → `http://localhost:3000` (with health check)
- `api.example.com/*` → `http://api-server:3000`
- `/static/*` → `http://cdn:8000`

## Create Your Config

Create `config.json`:

```json
{
  "listenAddr": ":8080",
  "routes": [
    {
      "match": {
        "pathPrefix": "/api"
      },
      "upstream": "http://api-service:3000",
      "stripPrefix": true,
      "addRequestHeaders": {
        "X-Forwarded-By": "reverse-proxy"
      },
      "healthCheck": {
        "path": "/health",
        "intervalMs": 10000
      }
    }
  ]
}
```

Then run:

```bash
./bin/reverse-proxy-go -config config.json
```

## Test the Proxy

```bash
# Forward request to matched route
curl http://localhost:8080/api/users

# Check routes
curl http://localhost:8080/_proxy/routes

# Check health
curl http://localhost:8080/_proxy/health
```

## Add/Remove Routes at Runtime

Add a new route:
```bash
curl -X POST http://localhost:8080/_proxy/routes \
  -H "Content-Type: application/json" \
  -d '{
    "match": { "pathPrefix": "/files" },
    "upstream": "http://file-service:3000",
    "stripPrefix": true
  }'
```

Remove a route (index 0):
```bash
curl -X DELETE http://localhost:8080/_proxy/routes/0
```

## Run Tests

```bash
make test
```

All 40+ tests pass with 73% coverage.

## Common Configurations

### Host-Based Routing
```json
{
  "match": { "host": "api.example.com" },
  "upstream": "http://api-backend:3000"
}
```

### Path-Based with Header Injection
```json
{
  "match": { "pathPrefix": "/internal" },
  "upstream": "http://internal-api:3000",
  "stripPrefix": true,
  "addRequestHeaders": {
    "X-Internal-Service": "true"
  }
}
```

### Security Headers Removal
```json
{
  "match": { "pathPrefix": "/public" },
  "upstream": "http://public-api:3000",
  "removeResponseHeaders": ["X-Internal-Version", "Server"]
}
```

### CORS Headers Addition
```json
{
  "match": { "pathPrefix": "/" },
  "upstream": "http://backend:3000",
  "addResponseHeaders": {
    "Access-Control-Allow-Origin": "*",
    "Access-Control-Allow-Methods": "GET, POST, PUT, DELETE"
  }
}
```

## Troubleshooting

### Route not matching
- Verify `match.host` or `match.pathPrefix` in config
- Check route order (first match wins)
- Test with `curl -v`

### Upstream returns 502
- Check if upstream is running: `curl {upstream}/health`
- If health check configured, verify health check path returns 2xx-3xx
- Review logs for "Bad Gateway" errors

### Health check not working
- Verify `healthCheck.path` exists on upstream
- Check `intervalMs` is not too aggressive
- Review GET /_proxy/health endpoint

### Headers not appearing
- Verify header names are spelled correctly
- Check request vs response headers (different config sections)
- Remember hop-by-hop headers are always stripped

## Docker Example

```dockerfile
FROM golang:1.22 AS builder
WORKDIR /app
COPY go.* ./
COPY cmd ./cmd
COPY internal ./internal
RUN go build -o reverse-proxy-go ./cmd/server

FROM alpine:latest
COPY --from=builder /app/reverse-proxy-go /
COPY config.json /
EXPOSE 8080
CMD ["/reverse-proxy-go", "-config", "/config.json"]
```

Build: `docker build -t reverse-proxy-go .`

Run: `docker run -p 8080:8080 -v $(pwd)/config.json:/config.json reverse-proxy-go`

## Production Checklist

- [ ] Test with your actual upstreams
- [ ] Set appropriate health check intervals (10-30 seconds typical)
- [ ] Configure request header additions for observability (X-Forwarded-For, etc.)
- [ ] Remove sensitive response headers (Server, X-Internal-*) 
- [ ] Monitor /_proxy/health endpoint
- [ ] Set up log aggregation for proxy errors
- [ ] Test graceful shutdown (Ctrl+C)
- [ ] Run under process supervisor (systemd, Docker, etc.)
- [ ] Place behind auth proxy if management API needs protection
