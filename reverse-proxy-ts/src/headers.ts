import type { IncomingHttpHeaders, OutgoingHttpHeaders } from "node:http";

/**
 * Hop-by-hop headers (RFC 7230 §6.1). `content-length` is end-to-end and is
 * forwarded so request and response bodies keep their framing.
 */
const HOP_BY_HOP = new Set([
  "connection",
  "keep-alive",
  "proxy-authenticate",
  "proxy-authorization",
  "te",
  "trailer",
  "transfer-encoding",
  "upgrade",
]);

export function isHopByHop(name: string): boolean {
  return HOP_BY_HOP.has(name.toLowerCase());
}

/**
 * Copy end-to-end headers, drop hop-by-hop names (including those listed in
 * `Connection`), apply removals, then apply additions. A name that is both
 * added and removed stays removed.
 */
export function rewriteHeaders(
  source: IncomingHttpHeaders,
  remove: readonly string[] | undefined,
  add: Readonly<Record<string, string>> | undefined,
): OutgoingHttpHeaders {
  const blocked = connectionHeaderNames(source.connection);
  for (const name of remove ?? []) blocked.add(name.toLowerCase());

  const headers: OutgoingHttpHeaders = {};
  for (const [name, value] of Object.entries(source)) {
    if (value === undefined) continue;
    const lower = name.toLowerCase();
    if (isHopByHop(lower) || blocked.has(lower)) continue;
    headers[lower] = value;
  }

  if (add) {
    for (const [name, value] of Object.entries(add)) {
      const lower = name.toLowerCase();
      if (isHopByHop(lower) || blocked.has(lower)) continue;
      headers[lower] = value;
    }
  }

  return headers;
}

function connectionHeaderNames(connection: string | string[] | undefined): Set<string> {
  const raw = Array.isArray(connection) ? connection.join(",") : (connection ?? "");
  const names = new Set<string>();
  for (const part of raw.split(",")) {
    const name = part.trim().toLowerCase();
    if (name.length > 0) names.add(name);
  }
  return names;
}
