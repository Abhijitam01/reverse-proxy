/** A route matches when every condition that is set is true. */
export interface RouteMatch {
  host?: string;
  pathPrefix?: string;
}

export interface HealthCheckConfig {
  path: string;
  /** Poll interval. Defaults to 10 seconds when omitted from JSON. */
  intervalMs: number;
}

export interface RouteConfig {
  match: RouteMatch;
  /** Absolute http(s) origin, optionally with a base path. */
  upstream: string;
  stripPrefix?: boolean;
  addRequestHeaders?: Record<string, string>;
  removeRequestHeaders?: string[];
  addResponseHeaders?: Record<string, string>;
  removeResponseHeaders?: string[];
  healthCheck?: HealthCheckConfig;
}

export interface ProxyConfig {
  /** `":3000"`, `"127.0.0.1:3000"`, or `"[::1]:3000"`. Defaults to `:3000`. */
  listenAddr?: string;
  routes: RouteConfig[];
}

export interface RouteView {
  index: number;
  match: RouteMatch;
  upstream: string;
  stripPrefix: boolean;
  addRequestHeaders: Record<string, string>;
  removeRequestHeaders: string[];
  addResponseHeaders: Record<string, string>;
  removeResponseHeaders: string[];
  healthCheck: HealthCheckConfig | null;
  isHealthy: boolean;
}
