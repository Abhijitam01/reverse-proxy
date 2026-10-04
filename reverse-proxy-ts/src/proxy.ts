import http from "node:http";
import https from "node:https";
import type { IncomingMessage, ServerResponse } from "node:http";
import { rewriteHeaders } from "./headers.js";
import { forwardPath } from "./router.js";
import { sendText } from "./respond.js";
import type { RouteConfig } from "./types.js";

const UPSTREAM_TIMEOUT_MS = 30_000;

export interface RequestTarget {
  pathname: string;
  search: string;
}

export function splitRequestTarget(url: string | undefined): RequestTarget | undefined {
  if (!url || !url.startsWith("/")) return undefined;
  const queryAt = url.indexOf("?");
  if (queryAt === -1) return { pathname: url, search: "" };
  return { pathname: url.slice(0, queryAt), search: url.slice(queryAt) };
}

export function upstreamUrl(route: RouteConfig, target: RequestTarget): URL {
  const url = new URL(route.upstream);
  const basePath = url.pathname === "/" ? "" : url.pathname.replace(/\/$/, "");
  const path = forwardPath(target.pathname, route);
  url.pathname = `${basePath}${path.startsWith("/") ? path : `/${path}`}`;
  url.search = target.search;
  url.hash = "";
  return url;
}

export function forwardRequest(
  req: IncomingMessage,
  res: ServerResponse,
  route: RouteConfig,
  target: RequestTarget,
  timeoutMs = UPSTREAM_TIMEOUT_MS,
): Promise<void> {
  return new Promise((resolve) => {
    let destination: URL;
    try {
      destination = upstreamUrl(route, target);
    } catch {
      sendText(res, 502, "Bad Gateway");
      resolve();
      return;
    }

    if (destination.protocol !== "http:" && destination.protocol !== "https:") {
      sendText(res, 502, "Bad Gateway");
      resolve();
      return;
    }

    const lib = destination.protocol === "https:" ? https : http;
    const headers = rewriteHeaders(
      req.headers,
      route.removeRequestHeaders,
      route.addRequestHeaders,
    );
    headers.host = destination.host;

    let settled = false;
    let timedOut = false;
    const finish = () => {
      if (settled) return;
      settled = true;
      upstreamReq.setTimeout(0);
      resolve();
    };

    const upstreamReq = lib.request(
      {
        protocol: destination.protocol,
        hostname: destination.hostname,
        port: destination.port,
        method: req.method,
        path: `${destination.pathname}${destination.search}`,
        headers,
      },
      (upstreamRes) => {
        const status = upstreamRes.statusCode ?? 502;
        const responseHeaders = rewriteHeaders(
          upstreamRes.headers,
          route.removeResponseHeaders,
          route.addResponseHeaders,
        );
        if (!res.headersSent) res.writeHead(status, responseHeaders);
        upstreamRes.pipe(res);
        upstreamRes.on("end", finish);
        upstreamRes.on("error", () => {
          if (!res.writableEnded) res.destroy();
          finish();
        });
      },
    );

    upstreamReq.setTimeout(timeoutMs, () => {
      timedOut = true;
      upstreamReq.destroy(new Error("upstream timeout"));
    });

    upstreamReq.on("error", () => {
      req.unpipe(upstreamReq);
      if (!res.headersSent) {
        sendText(res, timedOut ? 504 : 502, timedOut ? "Gateway Timeout" : "Bad Gateway");
        req.resume();
      } else if (!res.writableEnded) {
        res.destroy();
      }
      finish();
    });

    const abortUpstream = () => {
      upstreamReq.destroy();
    };
    req.on("aborted", abortUpstream);
    req.on("error", () => {
      abortUpstream();
      if (!res.headersSent) sendText(res, 400, "Bad Request");
      finish();
    });
    res.on("close", () => {
      if (!res.writableEnded) abortUpstream();
    });

    req.pipe(upstreamReq);
  });
}
