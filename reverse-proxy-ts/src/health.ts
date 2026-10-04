import http from "node:http";
import https from "node:https";
import type { HealthCheckConfig } from "./types.js";

const HEALTH_TIMEOUT_MS = 5_000;

interface Watch {
  refs: number;
  healthy: boolean;
  path: string;
  timer: NodeJS.Timeout;
  abort: AbortController | null;
}

export class HealthChecker {
  private readonly watches = new Map<string, Watch>();
  private closed = false;

  watch(upstream: string, check: HealthCheckConfig): void {
    if (this.closed) return;
    const existing = this.watches.get(upstream);
    if (existing) {
      existing.refs += 1;
      return;
    }

    const watch: Watch = {
      refs: 1,
      healthy: true,
      path: check.path,
      timer: setInterval(() => {
        void this.probe(upstream);
      }, check.intervalMs),
      abort: null,
    };
    watch.timer.unref();
    this.watches.set(upstream, watch);
    void this.probe(upstream);
  }

  unwatch(upstream: string): void {
    const watch = this.watches.get(upstream);
    if (!watch) return;
    watch.refs -= 1;
    if (watch.refs > 0) return;
    this.stop(upstream);
  }

  isHealthy(upstream: string): boolean {
    return this.watches.get(upstream)?.healthy ?? true;
  }

  snapshot(): Record<string, boolean> {
    const status: Record<string, boolean> = {};
    for (const [upstream, watch] of this.watches) {
      status[upstream] = watch.healthy;
    }
    return status;
  }

  close(): void {
    this.closed = true;
    for (const upstream of [...this.watches.keys()]) this.stop(upstream);
  }

  private stop(upstream: string): void {
    const watch = this.watches.get(upstream);
    if (!watch) return;
    clearInterval(watch.timer);
    watch.abort?.abort();
    this.watches.delete(upstream);
  }

  private async probe(upstream: string): Promise<void> {
    const watch = this.watches.get(upstream);
    if (!watch || this.closed) return;

    watch.abort?.abort();
    const abort = new AbortController();
    const timeout = setTimeout(() => abort.abort(), HEALTH_TIMEOUT_MS);
    timeout.unref();
    watch.abort = abort;

    const healthy = await probeUpstream(upstream, watch.path, abort.signal);
    clearTimeout(timeout);

    const current = this.watches.get(upstream);
    if (!current || current !== watch || watch.abort !== abort) return;
    current.healthy = healthy;
    current.abort = null;
  }
}

export function healthUrl(upstream: string, healthPath: string): URL {
  const url = new URL(upstream);
  const basePath = url.pathname === "/" ? "" : url.pathname.replace(/\/$/, "");
  const suffix = healthPath.startsWith("/") ? healthPath : `/${healthPath}`;
  url.pathname = `${basePath}${suffix}`;
  url.search = "";
  url.hash = "";
  return url;
}

async function probeUpstream(
  upstream: string,
  healthPath: string,
  signal: AbortSignal,
): Promise<boolean> {
  let url: URL;
  try {
    url = healthUrl(upstream, healthPath);
  } catch {
    return false;
  }

  const lib = url.protocol === "https:" ? https : http;
  return new Promise((resolve) => {
    const request = lib.get(
      {
        protocol: url.protocol,
        hostname: url.hostname,
        port: url.port,
        path: `${url.pathname}${url.search}`,
        headers: { host: url.host },
        signal,
      },
      (response) => {
        response.resume();
        const status = response.statusCode ?? 0;
        resolve(status >= 200 && status < 300);
      },
    );
    request.on("error", () => resolve(false));
  });
}
