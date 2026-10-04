import { ConfigError, loadConfig } from "../config.js";
import { createProxy, resolveListen } from "../server.js";

async function main(): Promise<void> {
  const configPath = process.argv[2] ?? "config.json";

  let config;
  try {
    config = loadConfig(configPath);
  } catch (error) {
    const message = error instanceof ConfigError ? error.message : "Failed to load configuration";
    console.error(message);
    process.exitCode = 1;
    return;
  }

  const proxy = createProxy(config);
  const listen = resolveListen(config);

  try {
    const port = await proxy.listen(listen.port, listen.host);
    console.log(
      `Reverse proxy listening on ${listen.host}:${port} (${config.routes.length} routes)`,
    );
  } catch (error) {
    const message = error instanceof Error ? error.message : "Failed to start";
    console.error(message);
    process.exitCode = 1;
    return;
  }

  const shutdown = (signal: string) => {
    console.log(`Received ${signal}, shutting down`);
    void proxy.close().then(
      () => process.exit(0),
      (error: unknown) => {
        console.error(error);
        process.exit(1);
      },
    );
  };

  process.once("SIGINT", () => shutdown("SIGINT"));
  process.once("SIGTERM", () => shutdown("SIGTERM"));
}

void main();
