# Reverse Proxy

> Part of the [BYO Everything](../README.md) portfolio — Reverse Proxy built from scratch in TypeScript, Go, and Rust.

## Implementations

| Language | Directory | Notes |
|----------|-----------|-------|
| TypeScript | [reverse-proxy-ts](./reverse-proxy-ts/) | Node.js 22, Fastify 5 |
| Go | [reverse-proxy-go](./reverse-proxy-go/) | Standard library + chi |
| Rust | [reverse-proxy-rust](./reverse-proxy-rust/) | tokio + axum |

## Build All

```bash
make build
make test
```

## Architecture

See [ARCHITECTURE.md](./ARCHITECTURE.md).
