# Reverse Proxy in Rust

A production-grade HTTP reverse proxy with host/path routing, prefix stripping, header manipulation, and upstream health checking.

## Features

- **Host-based routing**: Route requests based on Host header
- **Path-based routing**: Route requests based on URL path prefix
- **Prefix stripping**: Strip matched path prefix before forwarding to upstream
- **Header manipulation**: Add or remove request/response headers
- **Health checking**: Monitor upstream health with configurable health check endpoints
- **Management API**: Dynamic route management without restart
- **Hop-by-hop header filtering**: Proper HTTP protocol compliance

## Building

```bash
cargo build --release
```

## Running

```bash
./target/release/reverse-proxy
```

Server listens on `127.0.0.1:3000` by default.

## Configuration

Routes are configured via the management API or programmatically. Example configuration structure:

```rust
RouteConfig {
    match_: RouteMatch {
        host: Some("api.example.com".to_string()),
        path_prefix: Some("/v1".to_string()),
    },
    upstream: "http://backend:8080".to_string(),
    strip_prefix: Some(true),
    add_request_headers: Some(vec![
        ("x-forwarded-proto".to_string(), "https".to_string()),
    ].into_iter().collect()),
    remove_request_headers: Some(vec!["authorization".to_string()]),
    add_response_headers: None,
    remove_response_headers: None,
    health_check: Some(HealthCheckConfig {
        path: "/health".to_string(),
        interval_ms: 5000,
    }),
}
```

## Management API

### List Routes

```bash
GET /_proxy/routes
```

Returns all configured routes with health status.

### Add Route

```bash
POST /_proxy/routes
Content-Type: application/json

{
  "match": {
    "host": "api.example.com",
    "path_prefix": "/v1"
  },
  "upstream": "http://backend:8080",
  "strip_prefix": true
}
```

### Remove Route

```bash
DELETE /_proxy/routes/:index
```

### Health Status

```bash
GET /_proxy/health
```

Returns overall health status and per-upstream health.

## Project Structure

- `src/bin/server.rs` - Server entry point
- `src/lib.rs` - Library root
- `src/config.rs` - Configuration types
- `src/error.rs` - Error handling
- `src/router.rs` - Route matching logic
- `src/proxy.rs` - Request forwarding with hyper
- `src/health.rs` - Health check background tasks
- `src/routes.rs` - Axum route handlers
- `src/state.rs` - Application state
- `tests/proxy_test.rs` - Integration tests

## Testing

```bash
cargo test
```

Tests include:
- Route matching (host, path, combined)
- Header filtering (hop-by-hop removal, custom add/remove)
- Route management API
- Health check behavior
- Multiple routes with priority

All tests use real TCP servers to validate end-to-end proxy behavior.

## Design Principles

- **No unwrap() in production code**: All errors are properly handled
- **Immutable data**: Routes and configuration are cloned, never mutated in place
- **HTTP compliance**: Proper hop-by-hop header handling per RFC 7230
- **Graceful shutdown**: Health checks are cancelled via broadcast channel
- **Type safety**: Full compile-time safety with Rust's type system
