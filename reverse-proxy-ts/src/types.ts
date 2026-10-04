export interface RouteMatch {
  host?: string;
  pathPrefix?: string;
}

export interface HealthCheckConfig {
  path: string;
  intervalMs: number;
}

export interface RouteConfig {
  match: RouteMatch;
  upstream: string;
  stripPrefix?: boolean;
  addRequestHeaders?: Record<string, string>;
  removeRequestHeaders?: string[];
  addResponseHeaders?: Record<string, string>;
  removeResponseHeaders?: string[];
  healthCheck?: HealthCheckConfig;
}

export interface ProxyConfig {
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
