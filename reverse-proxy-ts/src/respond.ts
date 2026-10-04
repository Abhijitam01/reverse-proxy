import type { IncomingMessage, ServerResponse } from "node:http";

export const MAX_JSON_BYTES = 1_048_576;

export function sendText(res: ServerResponse, status: number, message: string): void {
  if (res.destroyed || res.headersSent || res.writableEnded) return;
  res.writeHead(status, { "content-type": "text/plain; charset=utf-8" });
  res.end(message);
}

export function sendJson(res: ServerResponse, status: number, body: unknown): void {
  if (res.destroyed || res.headersSent || res.writableEnded) return;
  const payload = JSON.stringify(body);
  res.writeHead(status, { "content-type": "application/json; charset=utf-8" });
  res.end(payload);
}

export function readJson(req: IncomingMessage, limit = MAX_JSON_BYTES): Promise<unknown> {
  return new Promise((resolve, reject) => {
    const chunks: Buffer[] = [];
    let size = 0;
    let settled = false;

    const fail = (error: Error) => {
      if (settled) return;
      settled = true;
      reject(error);
    };

    req.on("data", (chunk: Buffer) => {
      size += chunk.length;
      if (size > limit) {
        req.destroy();
        fail(new Error("Request body exceeds 1 MiB"));
        return;
      }
      chunks.push(chunk);
    });

    req.on("end", () => {
      if (settled) return;
      settled = true;
      const raw = Buffer.concat(chunks).toString("utf8");
      if (raw.trim() === "") {
        resolve({});
        return;
      }
      try {
        resolve(JSON.parse(raw) as unknown);
      } catch {
        reject(new Error("Request body is not valid JSON"));
      }
    });

    req.on("error", (error) => fail(error));
  });
}
