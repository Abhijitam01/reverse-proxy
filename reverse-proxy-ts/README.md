# Reverse Proxy (TypeScript)

HTTP reverse proxy on Node.js 22. Routing, header rewriting, and upstream health checks use the built-in `http` and `https` modules — there is no proxy library.

## What it does

Routes are checked in order. The first route whose conditions all match is used.

- `match.host` — `Host` header, compared case-insensitively. A value without a port matches that hostname on any port.
- `match.pathPrefix` — request path starts with this prefix. At least one of `host` or `pathPrefix` is required.
- `stripPrefix` — remove `pathPrefix` before forwarding. The upstream always receives an absolute path.
- Header rules run after hop-by-hop headers are removed (`Connection`, `Keep-Alive`, `Transfer-Encoding`, `TE`, `Trailer`, `Upgrade`, `Proxy-Authorization`, `Proxy-Authenticate`, plus any name listed in `Connection`). `Content-Length` is forwarded.
- A route with `healthCheck` polls `upstream + path` on one shared timer per upstream. A non-2xx response or a connection error marks that upstream unhealthy, and the proxy answers `502` until the probe succeeds again. Upstreams with no health check are treated as healthy.

No matching route is `404`. A connection failure is `502`. An upstream that accepts the connection but does not answer in time is `504`.

## Configuration

```json
{
  "listenAddr": ":3000",
  "routes": [
    {
      "match": { "pathPrefix": "/api" },
      "upstream": "http://127.0.0.1:4001",
      "stripPrefix": true,
      "addRequestHeaders": { "X-Forwarded-By": "reverse-proxy-ts" },
      "removeRequestHeaders": ["X-Internal"],
      "healthCheck": { "path": "/health", "intervalMs": 10000 }
    }
  ]
}
```

`listenAddr` accepts `:3000`, `127.0.0.1:3000`, or `[::1]:3000`. When it is omitted, `PORT` is used, then `3000`.

## Run

```bash
npm ci
npm run build
npm start -- config.example.json
```

`npm run dev -- config.json` runs TypeScript directly.

## Management API

| Method | Path | Result |
|--------|------|--------|
| GET | `/_proxy/routes` | Routes in order, with `isHealthy` |
| POST | `/_proxy/routes` | Add a route. `201` and the stored route |
| DELETE | `/_proxy/routes/:index` | Remove by index. `204` |
| GET | `/_proxy/health` | `{ "status": "healthy" \| "unhealthy", "routes": { "<upstream>": true } }` |

`/_proxy/*` is reserved and is never forwarded. `status` is `unhealthy` when any watched upstream is unhealthy. Routes that do not configure a health check are omitted from that map.

```bash
curl -X POST localhost:3000/_proxy/routes \
  -H 'content-type: application/json' \
  -d '{"match":{"pathPrefix":"/users"},"upstream":"http://127.0.0.1:4002","stripPrefix":true}'
```

## Tests

```bash
npm test
npm run typecheck
```

## Layout

| File | Role |
|------|------|
| `src/bin/server.ts` | Process entry, config path, shutdown |
| `src/config.ts` | Load and validate JSON |
| `src/router.ts` | First-match routing and prefix stripping |
| `src/headers.ts` | Hop-by-hop filtering and per-route header rules |
| `src/health.ts` | One background probe per upstream |
| `src/proxy.ts` | Stream the request and response |
| `src/server.ts` | HTTP server and management API |
