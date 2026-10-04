# Reverse Proxy Management API

## Overview

The reverse proxy provides a REST API for runtime management of routes without restart.

Base URL: `http://localhost:8080/_proxy`

## Endpoints

### 1. List Routes

**Request**
```
GET /_proxy/routes
```

**Response** (200 OK)
```json
{
  "routes": [
    {
      "index": 0,
      "match": {
        "host": "",
        "pathPrefix": "/api"
      },
      "upstream": "http://localhost:3000",
      "stripPrefix": true,
      "addRequestHeaders": {
        "X-Service": "api"
      },
      "removeRequestHeaders": [],
      "addResponseHeaders": null,
      "removeResponseHeaders": null,
      "healthCheck": {
        "path": "/health",
        "intervalMs": 10000
      },
      "isHealthy": true
    }
  ]
}
```

**cURL Example**
```bash
curl -s http://localhost:8080/_proxy/routes | jq .
```

---

### 2. Add Route

**Request**
```
POST /_proxy/routes
Content-Type: application/json

{
  "match": {
    "host": "api.example.com",
    "pathPrefix": "/users"
  },
  "upstream": "http://users-service:3000",
  "stripPrefix": true,
  "addRequestHeaders": {
    "X-Forwarded-For": "true"
  },
  "removeRequestHeaders": ["Authorization"],
  "addResponseHeaders": {
    "Cache-Control": "no-cache"
  },
  "removeResponseHeaders": ["Server"],
  "healthCheck": {
    "path": "/health",
    "intervalMs": 5000
  }
}
```

**Response** (201 Created)
```json
{
  "match": { "host": "api.example.com", "pathPrefix": "/users" },
  "upstream": "http://users-service:3000",
  "stripPrefix": true,
  "addRequestHeaders": { "X-Forwarded-For": "true" },
  "removeRequestHeaders": ["Authorization"],
  "addResponseHeaders": { "Cache-Control": "no-cache" },
  "removeResponseHeaders": ["Server"],
  "healthCheck": { "path": "/health", "intervalMs": 5000 }
}
```

**Validation Errors**

400 Bad Request - Missing upstream:
```json
{
  "error": "upstream is required"
}
```

400 Bad Request - Missing match criteria:
```json
{
  "error": "at least one of host or pathPrefix must be specified"
}
```

**cURL Example**
```bash
curl -X POST http://localhost:8080/_proxy/routes \
  -H "Content-Type: application/json" \
  -d '{
    "match": { "pathPrefix": "/api" },
    "upstream": "http://api-service:3000",
    "stripPrefix": true,
    "healthCheck": { "path": "/health", "intervalMs": 10000 }
  }'
```

---

### 3. Remove Route

**Request**
```
DELETE /_proxy/routes/{index}
```

Path Parameters:
- `index` (integer): Zero-based route index

**Response** (204 No Content)
```
```

**Error Responses**

400 Bad Request - Invalid index:
```json
{
  "error": "Invalid index"
}
```

404 Not Found - Index out of range:
```json
{
  "error": "Route not found"
}
```

**cURL Example**
```bash
# Remove route at index 0
curl -X DELETE http://localhost:8080/_proxy/routes/0

# Response: 204 No Content (no body)
```

---

### 4. Health Status

**Request**
```
GET /_proxy/health
```

**Response** (200 OK)
```json
{
  "status": "healthy",
  "routes": {
    "http://localhost:3000": true,
    "http://localhost:4000": false,
    "http://users-service:3000": true
  }
}
```

Status Values:
- `healthy`: At least one upstream is healthy (or no health checks configured)
- `unhealthy`: All upstreams with health checks are unhealthy

Routes Map:
- Key: Upstream URL
- Value: Boolean health status

**cURL Example**
```bash
curl -s http://localhost:8080/_proxy/health | jq .
```

---

## Main Proxy Handler

### Forward Request

**Request**
```
[ANY_METHOD] /path/to/endpoint
Host: example.com
```

**Routing Logic**
1. Iterate routes in order
2. Match request against route criteria (host AND/OR pathPrefix)
3. On match:
   - Check upstream health
   - If unhealthy: return 502 Bad Gateway
   - If healthy: forward request via httputil.ReverseProxy
4. On no match: return 404 Not Found

**Response**
```
[Status from upstream]
[Headers from upstream, with modifications]
[Body from upstream]
```

**Error Responses**

404 Not Found - No matching route:
```
Not Found
```

502 Bad Gateway - Upstream unhealthy:
```
Bad Gateway
```

**cURL Examples**
```bash
# Simple forward
curl http://localhost:8080/api/users

# With custom headers
curl -H "Authorization: Bearer token" http://localhost:8080/api/users

# POST with body
curl -X POST http://localhost:8080/api/users \
  -H "Content-Type: application/json" \
  -d '{"name": "John", "email": "john@example.com"}'
```

---

## Configuration Fields Reference

### Match Object
```json
{
  "host": "api.example.com",        // Optional: exact host match
  "pathPrefix": "/api"              // Optional: prefix match
}
```
At least one field must be specified.

### Upstream
```json
"upstream": "http://service:3000"   // Required: target URL (scheme + host + port)
```

### Prefix Stripping
```json
"stripPrefix": true                 // Optional (default: false): remove matched pathPrefix
```

Example:
- Request: `/api/users` with `pathPrefix: "/api"` and `stripPrefix: true`
- Forwarded: `/users`

### Headers
```json
"addRequestHeaders": {              // Optional: headers to add to requests
  "X-Custom": "value",
  "X-Service": "api"
},
"removeRequestHeaders": [           // Optional: header names to remove from requests
  "Authorization",
  "Cookie"
],
"addResponseHeaders": {             // Optional: headers to add to responses
  "Cache-Control": "public, max-age=3600"
},
"removeResponseHeaders": [          // Optional: header names to remove from responses
  "Server",
  "X-Internal-Version"
]
```

### Health Check
```json
"healthCheck": {                    // Optional: configure health monitoring
  "path": "/health",                // Required if healthCheck present: endpoint path
  "intervalMs": 10000               // Required if healthCheck present: check interval
}
```

Behavior:
- GET request to `{upstream}{path}` every `intervalMs` milliseconds
- Status 2xx-3xx: upstream is healthy
- Status 4xx-5xx or error: upstream is unhealthy
- If unhealthy: return 502 for requests to this route

---

## Complete Example: Adding Multiple Routes

```bash
# Add API route with health check
curl -X POST http://localhost:8080/_proxy/routes \
  -H "Content-Type: application/json" \
  -d '{
    "match": { "pathPrefix": "/api/users" },
    "upstream": "http://users-service:3000",
    "stripPrefix": true,
    "addRequestHeaders": { "X-Service": "users" },
    "healthCheck": { "path": "/health", "intervalMs": 10000 }
  }'

# Add web route without health check
curl -X POST http://localhost:8080/_proxy/routes \
  -H "Content-Type: application/json" \
  -d '{
    "match": { "pathPrefix": "/web" },
    "upstream": "http://web-service:3000",
    "stripPrefix": true
  }'

# Add host-based route
curl -X POST http://localhost:8080/_proxy/routes \
  -H "Content-Type: application/json" \
  -d '{
    "match": { "host": "admin.example.com" },
    "upstream": "http://admin-dashboard:3000",
    "addResponseHeaders": { "X-Protected": "true" }
  }'

# List all routes
curl http://localhost:8080/_proxy/routes | jq .

# Check health
curl http://localhost:8080/_proxy/health | jq .

# Remove first route
curl -X DELETE http://localhost:8080/_proxy/routes/0

# Verify removal
curl http://localhost:8080/_proxy/routes | jq '.routes | length'
```

---

## Error Handling

### Invalid JSON
Status: 400 Bad Request
```json
{
  "error": "Invalid request body: invalid character..."
}
```

### Missing Required Fields
Status: 400 Bad Request
```json
{
  "error": "upstream is required"
}
```

### Route Not Found
Status: 404 Not Found
```json
{
  "error": "Route not found"
}
```

### Invalid Route Index
Status: 400 Bad Request
```json
{
  "error": "Invalid index"
}
```

### Upstream Unhealthy
Status: 502 Bad Gateway
```
Bad Gateway
```

---

## Response Headers

All management API responses include:
```
Content-Type: application/json
```

Successful route operations include:
```
Status: 201 Created (POST)
Status: 204 No Content (DELETE)
Status: 200 OK (GET)
```

---

## Rate Limiting

Currently not implemented. Consider adding if deploying to production:
- Limit route additions per second
- Limit health check frequency
- Limit management API queries

---

## Security Notes

⚠️ Management API is **unauthenticated** by default.

Protection strategies:
1. Deploy behind authentication proxy
2. Bind to internal network only
3. Use firewall rules to restrict access
4. Wrap in application with auth middleware

Example with auth proxy:
```
[Client] → [Auth Proxy] → [Reverse Proxy Management API]
```
