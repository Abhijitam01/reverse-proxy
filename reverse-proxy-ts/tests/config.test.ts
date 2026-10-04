import { describe, expect, it } from "vitest";
import { ConfigError, loadConfig, parseConfig, parseListenAddr, parseRoute } from "../src/config.js";
import fs from "node:fs";
import os from "node:os";
import path from "node:path";

describe("parseConfig", () => {
  it("accepts a route with a path prefix and fills the health interval", () => {
    const config = parseConfig({
      listenAddr: ":3000",
      routes: [
        {
          match: { pathPrefix: "/api" },
          upstream: "http://127.0.0.1:4001",
          healthCheck: { path: "/health" },
        },
      ],
    });

    expect(config.listenAddr).toBe(":3000");
    expect(config.routes[0]?.healthCheck).toEqual({ path: "/health", intervalMs: 10_000 });
  });

  it("rejects a route that sets neither host nor pathPrefix", () => {
    expect(() => parseConfig({ routes: [{ match: {}, upstream: "http://127.0.0.1:1" }] })).toThrow(
      ConfigError,
    );
  });

  it("rejects an upstream that is not http(s)", () => {
    expect(() => parseRoute({ match: { pathPrefix: "/" }, upstream: "ftp://files.local" })).toThrow(
      /http or https/,
    );
  });

  it("rejects a path prefix that does not start with /", () => {
    expect(() => parseRoute({ match: { pathPrefix: "api" }, upstream: "http://127.0.0.1:1" })).toThrow(
      /pathPrefix/,
    );
  });
});

describe("loadConfig", () => {
  it("reads a JSON file from disk", () => {
    const dir = fs.mkdtempSync(path.join(os.tmpdir(), "proxy-config-"));
    const file = path.join(dir, "config.json");
    fs.writeFileSync(
      file,
      JSON.stringify({
        routes: [{ match: { host: "app.local" }, upstream: "http://127.0.0.1:4001" }],
      }),
    );

    expect(loadConfig(file).routes).toHaveLength(1);
  });

  it("reports a missing file", () => {
    expect(() => loadConfig("/tmp/does-not-exist-reverse-proxy.json")).toThrow(/not found/);
  });
});

describe("parseListenAddr", () => {
  it("parses a bare port, host:port, and bracketed IPv6", () => {
    expect(parseListenAddr(":3000")).toEqual({ host: "0.0.0.0", port: 3000 });
    expect(parseListenAddr("127.0.0.1:8080")).toEqual({ host: "127.0.0.1", port: 8080 });
    expect(parseListenAddr("[::1]:3000")).toEqual({ host: "::1", port: 3000 });
  });
});
