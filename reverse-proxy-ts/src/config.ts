import fs from "node:fs";
import path from "node:path";
import type { HealthCheckConfig, ProxyConfig, RouteConfig, RouteMatch } from "./types.js";

export const DEFAULT_HEALTH_INTERVAL_MS = 10_000;

export class ConfigError extends Error {
  constructor(message: string) {
    super(message);
    this.name = "ConfigError";
  }
}

export function loadConfig(filePath: string): ProxyConfig {
  const absolutePath = path.resolve(filePath);
  let content: string;
  try {
    content = fs.readFileSync(absolutePath, "utf8");
  } catch {
    throw new ConfigError(`Configuration file not found: ${absolutePath}`);
  }

  let parsed: unknown;
  try {
    parsed = JSON.parse(content);
  } catch (error) {
    const message = error instanceof Error ? error.message : "invalid JSON";
    throw new ConfigError(`Failed to parse configuration: ${message}`);
  }

  return parseConfig(parsed);
}

export function parseConfig(input: unknown): ProxyConfig {
  const record = asRecord(input, "Configuration must be an object");
  const routes = record.routes;
  if (!Array.isArray(routes)) {
    throw new ConfigError("Configuration must have a 'routes' array");
  }
  if (routes.length === 0) {
    throw new ConfigError("At least one route must be configured");
  }

  const config: ProxyConfig = {
    routes: routes.map((route, index) => parseRoute(route, `route ${index}`)),
  };

  if (record.listenAddr !== undefined) {
    if (typeof record.listenAddr !== "string" || record.listenAddr.length === 0) {
      throw new ConfigError("listenAddr must be a non-empty string");
    }
    config.listenAddr = record.listenAddr;
  }

  return config;
}

export function parseRoute(input: unknown, label = "route"): RouteConfig {
  const record = asRecord(input, `${label} must be an object`);
  const match = parseMatch(record.match, label);
  const upstream = parseUpstream(record.upstream, label);

  const route: RouteConfig = { match, upstream };

  if (record.stripPrefix !== undefined) {
    if (typeof record.stripPrefix !== "boolean") {
      throw new ConfigError(`${label}: stripPrefix must be a boolean`);
    }
    route.stripPrefix = record.stripPrefix;
  }

  const addRequestHeaders = parseHeaderMap(record.addRequestHeaders, label, "addRequestHeaders");
  if (addRequestHeaders) route.addRequestHeaders = addRequestHeaders;

  const removeRequestHeaders = parseHeaderList(
    record.removeRequestHeaders,
    label,
    "removeRequestHeaders",
  );
  if (removeRequestHeaders) route.removeRequestHeaders = removeRequestHeaders;

  const addResponseHeaders = parseHeaderMap(
    record.addResponseHeaders,
    label,
    "addResponseHeaders",
  );
  if (addResponseHeaders) route.addResponseHeaders = addResponseHeaders;

  const removeResponseHeaders = parseHeaderList(
    record.removeResponseHeaders,
    label,
    "removeResponseHeaders",
  );
  if (removeResponseHeaders) route.removeResponseHeaders = removeResponseHeaders;

  if (record.healthCheck !== undefined) {
    route.healthCheck = parseHealthCheck(record.healthCheck, label);
  }

  return route;
}

export interface ListenAddress {
  host: string;
  port: number;
}

export function parseListenAddr(addr: string): ListenAddress {
  let host = "0.0.0.0";
  let portText = addr;

  if (addr.startsWith("[")) {
    const end = addr.indexOf("]");
    if (end === -1) throw new ConfigError(`Invalid listen address: ${addr}`);
    host = addr.slice(1, end);
    const rest = addr.slice(end + 1);
    if (rest === "") throw new ConfigError(`Invalid listen address: ${addr}`);
    if (!rest.startsWith(":")) throw new ConfigError(`Invalid listen address: ${addr}`);
    portText = rest.slice(1);
  } else if (addr.startsWith(":")) {
    portText = addr.slice(1);
  } else {
    const separator = addr.lastIndexOf(":");
    if (separator !== -1) {
      host = addr.slice(0, separator);
      portText = addr.slice(separator + 1);
    }
  }

  if (!/^[0-9]+$/.test(portText)) {
    throw new ConfigError(`Invalid listen port in ${addr}`);
  }
  const port = Number(portText);
  if (port > 65535) throw new ConfigError(`Invalid listen port in ${addr}`);
  return { host, port };
}

function parseMatch(input: unknown, label: string): RouteMatch {
  const record = asRecord(input, `${label}: match must be an object`);
  const match: RouteMatch = {};

  if (record.host !== undefined) {
    if (typeof record.host !== "string" || record.host.length === 0) {
      throw new ConfigError(`${label}: match.host must be a non-empty string`);
    }
    match.host = record.host;
  }

  if (record.pathPrefix !== undefined) {
    if (typeof record.pathPrefix !== "string" || !record.pathPrefix.startsWith("/")) {
      throw new ConfigError(`${label}: match.pathPrefix must be a string starting with '/'`);
    }
    match.pathPrefix = record.pathPrefix;
  }

  if (!match.host && !match.pathPrefix) {
    throw new ConfigError(`${label}: match must set host, pathPrefix, or both`);
  }

  return match;
}

function parseUpstream(input: unknown, label: string): string {
  if (typeof input !== "string" || input.length === 0) {
    throw new ConfigError(`${label}: upstream is required`);
  }
  let url: URL;
  try {
    url = new URL(input);
  } catch {
    throw new ConfigError(`${label}: upstream must be an absolute URL`);
  }
  if (url.protocol !== "http:" && url.protocol !== "https:") {
    throw new ConfigError(`${label}: upstream must use http or https`);
  }
  if (!url.hostname) {
    throw new ConfigError(`${label}: upstream must include a host`);
  }
  return input;
}

function parseHealthCheck(input: unknown, label: string): HealthCheckConfig {
  const record = asRecord(input, `${label}: healthCheck must be an object`);
  if (typeof record.path !== "string" || !record.path.startsWith("/")) {
    throw new ConfigError(`${label}: healthCheck.path must be a string starting with '/'`);
  }

  let intervalMs = DEFAULT_HEALTH_INTERVAL_MS;
  if (record.intervalMs !== undefined) {
    if (
      typeof record.intervalMs !== "number" ||
      !Number.isInteger(record.intervalMs) ||
      record.intervalMs <= 0
    ) {
      throw new ConfigError(`${label}: healthCheck.intervalMs must be a positive integer`);
    }
    intervalMs = record.intervalMs;
  }

  return { path: record.path, intervalMs };
}

function parseHeaderMap(
  input: unknown,
  label: string,
  field: string,
): Record<string, string> | undefined {
  if (input === undefined) return undefined;
  const record = asRecord(input, `${label}: ${field} must be an object`);
  const headers: Record<string, string> = {};
  for (const [name, value] of Object.entries(record)) {
    if (typeof value !== "string") {
      throw new ConfigError(`${label}: ${field}.${name} must be a string`);
    }
    if (name.length === 0) {
      throw new ConfigError(`${label}: ${field} contains an empty header name`);
    }
    headers[name] = value;
  }
  return headers;
}

function parseHeaderList(input: unknown, label: string, field: string): string[] | undefined {
  if (input === undefined) return undefined;
  if (!Array.isArray(input)) {
    throw new ConfigError(`${label}: ${field} must be an array of header names`);
  }
  const names: string[] = [];
  for (const item of input) {
    if (typeof item !== "string" || item.length === 0) {
      throw new ConfigError(`${label}: ${field} must be an array of header names`);
    }
    names.push(item);
  }
  return names;
}

function asRecord(input: unknown, message: string): Record<string, unknown> {
  if (!input || typeof input !== "object" || Array.isArray(input)) {
    throw new ConfigError(message);
  }
  return input as Record<string, unknown>;
}
