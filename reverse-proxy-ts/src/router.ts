import type { HealthChecker } from "./health.js";
import type { RouteConfig } from "./types.js";

/**
 * First match wins. A route matches when every condition it sets is true.
 * `pathPrefix` is a raw prefix: `/api` also matches `/apiv2`. Put longer
 * prefixes first when routes overlap.
 */
export function matchRoute(
  routes: readonly RouteConfig[],
  host: string | undefined,
  path: string,
): RouteConfig | undefined {
  return routes.find((route) => routeMatches(route, host, path));
}

export function routeMatches(route: RouteConfig, host: string | undefined, path: string): boolean {
  const expectedHost = route.match.host;
  const pathPrefix = route.match.pathPrefix;
  if (!expectedHost && !pathPrefix) return false;
  if (expectedHost && !hostMatches(host, expectedHost)) return false;
  if (pathPrefix && !path.startsWith(pathPrefix)) return false;
  return true;
}

/**
 * Host comparison is case-insensitive. A pattern without a port matches the
 * hostname only, so `app.local` matches `app.local:3000`.
 */
export function hostMatches(actual: string | undefined, expected: string): boolean {
  if (!actual) return false;
  const left = actual.toLowerCase();
  const right = expected.toLowerCase();
  if (left === right) return true;
  // `app.local` matches `app.local:3000`. A pattern that includes a port must match exactly.
  return stripPort(left) === right;
}

export function forwardPath(path: string, route: RouteConfig): string {
  const prefix = route.match.pathPrefix;
  if (!route.stripPrefix || !prefix || !path.startsWith(prefix)) return path;

  const rest = path.slice(prefix.length);
  if (rest === "") return "/";
  return rest.startsWith("/") ? rest : `/${rest}`;
}

export class RouteTable {
  private routes: RouteConfig[] = [];

  constructor(
    private readonly health: HealthChecker,
    initial: readonly RouteConfig[] = [],
  ) {
    for (const route of initial) this.add(route);
  }

  match(host: string | undefined, path: string): RouteConfig | undefined {
    return matchRoute(this.routes, host, path);
  }

  list(): RouteConfig[] {
    return this.routes.slice();
  }

  add(route: RouteConfig): number {
    if (route.healthCheck) this.health.watch(route.upstream, route.healthCheck);
    this.routes = [...this.routes, route];
    return this.routes.length - 1;
  }

  remove(index: number): RouteConfig | undefined {
    const removed = this.routes[index];
    if (!removed) return undefined;
    this.routes = this.routes.filter((_, i) => i !== index);
    if (removed.healthCheck) this.health.unwatch(removed.upstream);
    return removed;
  }
}

function stripPort(host: string): string {
  if (host.startsWith("[")) {
    const end = host.indexOf("]");
    return end === -1 ? host : host.slice(0, end + 1);
  }
  return host.replace(/:\d+$/, "");
}
