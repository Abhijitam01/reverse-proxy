import http from "node:http";
import { afterEach, describe, expect, it } from "vitest";
import { createProxy } from "../src/server.js";
import type { ProxyConfig, RouteConfig } from "../src/types.js";
import { closeServer, listen, request, waitFor } from "./http.js";

const stop: Array<() => Promise<void>> = [];

afterEach(async () => {
  for (const close of stop.splice(0).reverse()) await close();
});

describe("reverse proxy", () => {
  it("forwards a request and strips the path prefix", async () => {
    const upstream = await startUpstream();
    const proxy = await startProxy([
      { match: { pathPrefix: "/api" }, upstream: upstream.url, stripPrefix: true },
    ]);

    const response = await request({ port: proxy.port, path: "/api/users?page=2" });
    const echoed = JSON.parse(response.body) as { path: string };

    expect(response.status).toBe(200);
    expect(echoed.path).toBe("/users?page=2");
  });

  it("joins an upstream base path with the stripped request path", async () => {
    const upstream = await startUpstream();
    const proxy = await startProxy([
      {
        match: { pathPrefix: "/api" },
        upstream: `${upstream.url}/base`,
        stripPrefix: true,
      },
    ]);

    const response = await request({ port: proxy.port, path: "/api/users" });
    expect(JSON.parse(response.body)).toMatchObject({ path: "/base/users" });
  });

  it("routes on host and keeps the original path", async () => {
    const upstream = await startUpstream();
    const proxy = await startProxy([{ match: { host: "app.local" }, upstream: upstream.url }]);

    const matched = await request({
      port: proxy.port,
      path: "/assets/app.js",
      headers: { host: "app.local" },
    });
    const missed = await request({
      port: proxy.port,
      path: "/assets/app.js",
      headers: { host: "other.local" },
    });

    expect(matched.status).toBe(200);
    expect(JSON.parse(matched.body)).toMatchObject({ path: "/assets/app.js" });
    expect(missed.status).toBe(404);
  });

  it("uses the first matching route", async () => {
    const first = await startUpstream();
    const second = await startUpstream();
    const proxy = await startProxy([
      { match: { pathPrefix: "/api" }, upstream: first.url },
      { match: { pathPrefix: "/" }, upstream: second.url },
    ]);

    const response = await request({ port: proxy.port, path: "/api/test" });
    expect(JSON.parse(response.body)).toMatchObject({ id: first.id });
  });

  it("adds and removes request and response headers", async () => {
    const upstream = await startUpstream();
    const proxy = await startProxy([
      {
        match: { pathPrefix: "/" },
        upstream: upstream.url,
        addRequestHeaders: { "X-Proxy": "ts" },
        removeRequestHeaders: ["X-Secret"],
        addResponseHeaders: { "X-Added": "yes" },
        removeResponseHeaders: ["X-Upstream-Internal"],
      },
    ]);

    const response = await request({
      port: proxy.port,
      path: "/",
      headers: {
        "x-secret": "nope",
        "x-public": "keep",
        connection: "close, x-drop-me",
        "x-drop-me": "secret",
        "proxy-authorization": "Basic abc",
      },
    });
    const echoed = JSON.parse(response.body) as { headers: Record<string, string> };

    expect(echoed.headers["x-proxy"]).toBe("ts");
    expect(echoed.headers["x-public"]).toBe("keep");
    expect(echoed.headers["x-secret"]).toBeUndefined();
    expect(echoed.headers["x-drop-me"]).toBeUndefined();
    expect(echoed.headers["proxy-authorization"]).toBeUndefined();
    expect(response.headers["x-added"]).toBe("yes");
    expect(response.headers["x-upstream"]).toBe("yes");
    expect(response.headers["x-upstream-internal"]).toBeUndefined();
  });

  it("passes the request body through and preserves the upstream status", async () => {
    const upstream = await startUpstream();
    const proxy = await startProxy([{ match: { pathPrefix: "/api" }, upstream: upstream.url }]);

    const response = await request({
      port: proxy.port,
      method: "POST",
      path: "/api/data",
      headers: { "content-type": "application/json" },
      body: JSON.stringify({ name: "ada" }),
    });

    expect(response.status).toBe(201);
    expect(JSON.parse(response.body)).toMatchObject({ body: JSON.stringify({ name: "ada" }) });
  });

  it("returns 502 when the upstream connection fails", async () => {
    const proxy = await startProxy([
      { match: { pathPrefix: "/api" }, upstream: "http://127.0.0.1:1" },
    ]);

    const response = await request({ port: proxy.port, path: "/api/users" });
    expect(response.status).toBe(502);
    expect(response.body).toBe("Bad Gateway");
  });

  it("returns 504 when the upstream does not respond in time", async () => {
    const hanging = http.createServer(() => {});
    const port = await listen(hanging);
    stop.push(() => closeServer(hanging));

    const proxy = await startProxy(
      [{ match: { pathPrefix: "/" }, upstream: `http://127.0.0.1:${port}` }],
      200,
    );

    const response = await request({ port: proxy.port, path: "/slow" });
    expect(response.status).toBe(504);
  });

  it("rejects traffic to an upstream that fails its health check, then recovers", async () => {
    let healthy = false;
    let appHits = 0;
    const upstream = http.createServer((req, res) => {
      if (req.url === "/health") {
        res.writeHead(healthy ? 200 : 500);
        res.end(healthy ? "ok" : "down");
        return;
      }
      appHits += 1;
      res.writeHead(200, { "content-type": "text/plain" });
      res.end("ready");
    });
    const port = await listen(upstream);
    stop.push(() => closeServer(upstream));

    const proxy = await startProxy([
      {
        match: { pathPrefix: "/api" },
        upstream: `http://127.0.0.1:${port}`,
        healthCheck: { path: "/health", intervalMs: 30 },
      },
    ]);

    await waitFor(async () => {
      const status = await request({ port: proxy.port, path: "/_proxy/health" });
      const payload = JSON.parse(status.body) as { routes: Record<string, boolean> };
      return Object.values(payload.routes).includes(false);
    });

    const blocked = await request({ port: proxy.port, path: "/api/users" });
    expect(blocked.status).toBe(502);
    expect(appHits).toBe(0);

    healthy = true;
    await waitFor(async () => {
      const status = await request({ port: proxy.port, path: "/_proxy/health" });
      const payload = JSON.parse(status.body) as { status: string };
      return payload.status === "healthy";
    });

    const allowed = await request({ port: proxy.port, path: "/api/users" });
    expect(allowed.status).toBe(200);
    expect(allowed.body).toBe("ready");
    expect(appHits).toBe(1);
  });

  it("lists, adds, and removes routes without restarting", async () => {
    const upstream = await startUpstream();
    const proxy = await startProxy([
      { match: { pathPrefix: "/api" }, upstream: upstream.url, stripPrefix: true },
    ]);

    const listed = await request({ port: proxy.port, path: "/_proxy/routes" });
    expect(JSON.parse(listed.body)).toMatchObject({
      routes: [{ index: 0, upstream: upstream.url, isHealthy: true, stripPrefix: true }],
    });

    const created = await request({
      port: proxy.port,
      method: "POST",
      path: "/_proxy/routes",
      headers: { "content-type": "application/json" },
      body: JSON.stringify({
        match: { host: "files.local" },
        upstream: upstream.url,
      }),
    });
    expect(created.status).toBe(201);

    const viaHost = await request({
      port: proxy.port,
      path: "/file",
      headers: { host: "files.local" },
    });
    expect(viaHost.status).toBe(200);

    const removed = await request({ port: proxy.port, method: "DELETE", path: "/_proxy/routes/1" });
    expect(removed.status).toBe(204);

    const after = await request({
      port: proxy.port,
      path: "/file",
      headers: { host: "files.local" },
    });
    expect(after.status).toBe(404);

    const invalid = await request({
      port: proxy.port,
      method: "POST",
      path: "/_proxy/routes",
      headers: { "content-type": "application/json" },
      body: JSON.stringify({ upstream: "http://127.0.0.1:9" }),
    });
    expect(invalid.status).toBe(400);
  });

  it("keeps a shared upstream probe until the last route is removed", async () => {
    const upstream = await startUpstream();
    const proxy = await startProxy([
      {
        match: { pathPrefix: "/a" },
        upstream: upstream.url,
        healthCheck: { path: "/health", intervalMs: 50 },
      },
      {
        match: { pathPrefix: "/b" },
        upstream: upstream.url,
        healthCheck: { path: "/health", intervalMs: 50 },
      },
    ]);

    await waitFor(async () => {
      const status = await request({ port: proxy.port, path: "/_proxy/health" });
      const payload = JSON.parse(status.body) as { routes: Record<string, boolean> };
      return payload.routes[upstream.url] === true;
    });

    expect((await request({ port: proxy.port, method: "DELETE", path: "/_proxy/routes/0" })).status).toBe(
      204,
    );

    const stillWatched = JSON.parse(
      (await request({ port: proxy.port, path: "/_proxy/health" })).body,
    ) as { routes: Record<string, boolean> };
    expect(stillWatched.routes[upstream.url]).toBe(true);

    await request({ port: proxy.port, method: "DELETE", path: "/_proxy/routes/0" });
    const cleared = JSON.parse((await request({ port: proxy.port, path: "/_proxy/health" })).body) as {
      status: string;
      routes: Record<string, boolean>;
    };
    expect(cleared.status).toBe("healthy");
    expect(cleared.routes).toEqual({});
  });

  it("serves the management API ahead of a catch-all route", async () => {
    const upstream = await startUpstream();
    const proxy = await startProxy([{ match: { pathPrefix: "/" }, upstream: upstream.url }]);

    const response = await request({ port: proxy.port, path: "/_proxy/health" });
    expect(JSON.parse(response.body)).toMatchObject({ status: "healthy", routes: {} });
  });
});

async function startUpstream(): Promise<{ url: string; id: number }> {
  const id = Math.floor(Math.random() * 1_000_000);
  const server = http.createServer((req, res) => {
    const chunks: Buffer[] = [];
    req.on("data", (chunk: Buffer) => chunks.push(chunk));
    req.on("end", () => {
      const status = req.method === "POST" ? 201 : 200;
      const body = JSON.stringify({
        id,
        method: req.method,
        path: req.url,
        headers: req.headers,
        body: Buffer.concat(chunks).toString("utf8"),
      });
      res.writeHead(status, {
        "content-type": "application/json",
        "x-upstream": "yes",
        "x-upstream-internal": "hidden",
      });
      res.end(body);
    });
  });
  const port = await listen(server);
  stop.push(() => closeServer(server));
  return { url: `http://127.0.0.1:${port}`, id };
}

async function startProxy(routes: RouteConfig[], upstreamTimeoutMs?: number): Promise<{ port: number }> {
  const config: ProxyConfig = { routes };
  const proxy = createProxy(config, upstreamTimeoutMs ? { upstreamTimeoutMs } : {});
  const port = await proxy.listen(0, "127.0.0.1");
  stop.push(() => proxy.close());
  return { port };
}
