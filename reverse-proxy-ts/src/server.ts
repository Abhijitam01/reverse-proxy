import http from "node:http";
import type { IncomingMessage, ServerResponse } from "node:http";
import { ConfigError, parseListenAddr, parseRoute, type ListenAddress } from "./config.js";
import { HealthChecker } from "./health.js";
import { forwardRequest, splitRequestTarget } from "./proxy.js";
import { readJson, sendJson, sendText } from "./respond.js";
import { RouteTable } from "./router.js";
import type { ProxyConfig, RouteConfig, RouteView } from "./types.js";

const ROUTES_PATH = "/_proxy/routes";
const HEALTH_PATH = "/_proxy/health";

export interface ProxyServer {
  readonly server: http.Server;
  listen(port?: number, host?: string): Promise<number>;
  close(): Promise<void>;
}

export interface ProxyOptions {
  upstreamTimeoutMs?: number;
}

export function createProxy(config: ProxyConfig, options: ProxyOptions = {}): ProxyServer {
  const health = new HealthChecker();
  const routes = new RouteTable(health, config.routes);
  const upstreamTimeoutMs = options.upstreamTimeoutMs ?? 30_000;
  let closed = false;

  const server = http.createServer((req, res) => {
    void handleRequest(req, res, routes, health, upstreamTimeoutMs).catch((error: unknown) => {
      const message = error instanceof Error ? error.message : "Internal Server Error";
      console.error("proxy error:", message);
      sendText(res, 500, "Internal Server Error");
    });
  });

  return {
    server,
    listen(port = 3000, host = "0.0.0.0") {
      return new Promise((resolve, reject) => {
        const onError = (error: Error) => {
          server.off("error", onError);
          reject(error);
        };
        server.once("error", onError);
        server.listen(port, host, () => {
          server.off("error", onError);
          const address = server.address();
          resolve(typeof address === "object" && address ? address.port : port);
        });
      });
    },
    close() {
      if (closed) return Promise.resolve();
      closed = true;
      health.close();
      if (!server.listening) return Promise.resolve();
      server.closeAllConnections();
      return new Promise((resolve, reject) => {
        server.close((error) => (error ? reject(error) : resolve()));
      });
    },
  };
}

export function resolveListen(config: ProxyConfig, envPort = process.env.PORT): ListenAddress {
  if (config.listenAddr) return parseListenAddr(config.listenAddr);
  if (envPort && envPort.length > 0) return parseListenAddr(`:${envPort}`);
  return { host: "0.0.0.0", port: 3000 };
}

async function handleRequest(
  req: IncomingMessage,
  res: ServerResponse,
  routes: RouteTable,
  health: HealthChecker,
  upstreamTimeoutMs: number,
): Promise<void> {
  const target = splitRequestTarget(req.url);
  if (!target) {
    sendText(res, 400, "Bad Request");
    return;
  }

  if (target.pathname === HEALTH_PATH && req.method === "GET") {
    sendJson(res, 200, healthPayload(health));
    return;
  }

  if (target.pathname === ROUTES_PATH && req.method === "GET") {
    sendJson(res, 200, { routes: listRoutes(routes, health) });
    return;
  }

  if (target.pathname === ROUTES_PATH && req.method === "POST") {
    await addRoute(req, res, routes);
    return;
  }

  if (req.method === "DELETE" && target.pathname.startsWith(`${ROUTES_PATH}/`)) {
    removeRoute(target.pathname, res, routes);
    return;
  }

  const route = routes.match(req.headers.host, target.pathname);
  if (!route) {
    sendText(res, 404, "Not Found");
    return;
  }

  if (!health.isHealthy(route.upstream)) {
    sendText(res, 502, "Bad Gateway");
    return;
  }

  await forwardRequest(req, res, route, target, upstreamTimeoutMs);
}

function healthPayload(health: HealthChecker): {
  status: "healthy" | "unhealthy";
  routes: Record<string, boolean>;
} {
  const upstreams = health.snapshot();
  const status = Object.values(upstreams).every((healthy) => healthy) ? "healthy" : "unhealthy";
  return { status, routes: upstreams };
}

function listRoutes(routes: RouteTable, health: HealthChecker): RouteView[] {
  return routes.list().map((route, index) => ({
    index,
    match: route.match,
    upstream: route.upstream,
    stripPrefix: route.stripPrefix ?? false,
    addRequestHeaders: route.addRequestHeaders ?? {},
    removeRequestHeaders: route.removeRequestHeaders ?? [],
    addResponseHeaders: route.addResponseHeaders ?? {},
    removeResponseHeaders: route.removeResponseHeaders ?? [],
    healthCheck: route.healthCheck ?? null,
    isHealthy: health.isHealthy(route.upstream),
  }));
}

async function addRoute(
  req: IncomingMessage,
  res: ServerResponse,
  routes: RouteTable,
): Promise<void> {
  let body: unknown;
  try {
    body = await readJson(req);
  } catch (error) {
    const message = error instanceof Error ? error.message : "Invalid request body";
    sendText(res, 400, message);
    return;
  }

  let route: RouteConfig;
  try {
    route = parseRoute(body);
  } catch (error) {
    const message = error instanceof ConfigError ? error.message : "Invalid route";
    sendText(res, 400, message);
    return;
  }

  routes.add(route);
  sendJson(res, 201, route);
}

function removeRoute(pathname: string, res: ServerResponse, routes: RouteTable): void {
  const raw = pathname.slice(`${ROUTES_PATH}/`.length);
  if (!/^[0-9]+$/.test(raw)) {
    sendText(res, 400, "Invalid index");
    return;
  }
  const index = Number(raw);
  if (!routes.remove(index)) {
    sendText(res, 404, "Route not found");
    return;
  }
  res.writeHead(204);
  res.end();
}
