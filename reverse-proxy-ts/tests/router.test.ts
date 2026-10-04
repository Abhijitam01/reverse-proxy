import { describe, expect, it } from "vitest";
import { rewriteHeaders } from "../src/headers.js";
import { forwardPath, hostMatches, matchRoute } from "../src/router.js";
import type { RouteConfig } from "../src/types.js";

const route = (partial: Partial<RouteConfig> & Pick<RouteConfig, "match" | "upstream">): RouteConfig =>
  partial;

describe("matchRoute", () => {
  const routes: RouteConfig[] = [
    route({ match: { host: "api.local", pathPrefix: "/v1" }, upstream: "http://api" }),
    route({ match: { pathPrefix: "/api" }, upstream: "http://prefix" }),
    route({ match: { host: "static.local" }, upstream: "http://static" }),
  ];

  it("requires every condition that is set", () => {
    expect(matchRoute(routes, "api.local", "/v1/users")?.upstream).toBe("http://api");
    expect(matchRoute(routes, "api.local", "/other")?.upstream).toBeUndefined();
    expect(matchRoute(routes, "other.local", "/v1/users")?.upstream).toBeUndefined();
  });

  it("uses configuration order, not the longest prefix", () => {
    expect(matchRoute(routes, undefined, "/api/users")?.upstream).toBe("http://prefix");
    expect(matchRoute(routes, "static.local", "/anything")?.upstream).toBe("http://static");
  });

  it("matches a host with or without the port", () => {
    expect(hostMatches("App.Local:3000", "app.local")).toBe(true);
    expect(hostMatches("app.local:3000", "app.local:4000")).toBe(false);
    expect(hostMatches("[::1]:3000", "::1")).toBe(false);
    expect(hostMatches("[::1]:3000", "[::1]")).toBe(true);
  });
});

describe("forwardPath", () => {
  it("strips the matched prefix and keeps an absolute path", () => {
    const api = route({ match: { pathPrefix: "/api" }, upstream: "http://api", stripPrefix: true });
    expect(forwardPath("/api/users", api)).toBe("/users");
    expect(forwardPath("/api", api)).toBe("/");
  });

  it("leaves the path unchanged when stripPrefix is off", () => {
    const api = route({ match: { pathPrefix: "/api" }, upstream: "http://api" });
    expect(forwardPath("/api/users", api)).toBe("/api/users");
  });
});

describe("rewriteHeaders", () => {
  it("drops hop-by-hop headers and headers named by Connection", () => {
    const headers = rewriteHeaders(
      {
        connection: "close, x-drop-me",
        "keep-alive": "timeout=5",
        "proxy-authorization": "Basic abc",
        "x-drop-me": "secret",
        "x-keep": "yes",
        "content-length": "4",
      },
      ["x-keep"],
      { "X-Added": "1", "Proxy-Authorization": "nope" },
    );

    expect(headers.connection).toBeUndefined();
    expect(headers["keep-alive"]).toBeUndefined();
    expect(headers["proxy-authorization"]).toBeUndefined();
    expect(headers["x-drop-me"]).toBeUndefined();
    expect(headers["x-keep"]).toBeUndefined();
    expect(headers["content-length"]).toBe("4");
    expect(headers["x-added"]).toBe("1");
  });
});
