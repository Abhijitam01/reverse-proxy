"""Health checking for upstream servers."""

from __future__ import annotations

import asyncio
import http.client
import urllib.parse


class HealthChecker:
    def __init__(self):
        self._health: dict[str, dict] = {}
        self._tasks: dict[str, asyncio.Task] = {}

    def register_route(self, route: dict) -> None:
        upstream = route["upstream"]
        if upstream not in self._health:
            self._health[upstream] = {"url": upstream, "healthy": True}
        if route.get("healthCheck"):
            self._start_health_check(route)

    def unregister_route(self, route: dict) -> None:
        upstream = route["upstream"]
        task = self._tasks.pop(upstream, None)
        if task and not task.done():
            task.cancel()

    def is_healthy(self, upstream: str) -> bool:
        return self._health.get(upstream, {"healthy": True})["healthy"]

    def get_health(self, upstream: str) -> dict:
        return self._health.get(upstream, {"url": upstream, "healthy": True})

    def get_all_health(self) -> dict[str, dict]:
        return dict(self._health)

    async def check_upstream_health(self, upstream: str, health_path: str) -> bool:
        try:
            url = urllib.parse.urljoin(upstream, health_path)
            parsed = urllib.parse.urlparse(url)
            # use asyncio to run blocking http in thread
            def _fetch():
                try:
                    conn_cls = http.client.HTTPSConnection if parsed.scheme == "https" else http.client.HTTPConnection
                    host = parsed.hostname
                    port = parsed.port
                    conn = conn_cls(host, port, timeout=5) if port else conn_cls(host, timeout=5)
                    path = parsed.path or "/"
                    if parsed.query:
                        path += "?" + parsed.query
                    conn.request("GET", path)
                    resp = conn.getresponse()
                    status = resp.status
                    conn.close()
                    return 200 <= status < 300
                except Exception:
                    return False
            return await asyncio.to_thread(_fetch)
        except Exception:
            return False

    def _start_health_check(self, route: dict) -> None:
        upstream = route["upstream"]
        if upstream in self._tasks:
            return
        hc = route["healthCheck"]
        interval = hc.get("intervalMs", 10000) / 1000

        async def loop():
            await self._perform_check(route)
            while True:
                await asyncio.sleep(interval)
                await self._perform_check(route)

        try:
            loop_task = asyncio.create_task(loop())
            self._tasks[upstream] = loop_task
        except RuntimeError:
            # no running loop yet, will start later - store config
            pass

    async def _perform_check(self, route: dict) -> None:
        hc = route.get("healthCheck")
        if not hc:
            return
        upstream = route["upstream"]
        try:
            healthy = await self.check_upstream_health(upstream, hc["path"])
            cur = self._health.get(upstream, {"url": upstream, "healthy": True})
            cur["healthy"] = healthy
            cur["lastCheck"] = int(asyncio.get_event_loop().time() * 1000) if asyncio.get_event_loop().is_running() else 0
            # need real time
            import time
            cur["lastCheck"] = int(time.time() * 1000)
            cur.pop("error", None)
            self._health[upstream] = cur
        except Exception as e:
            cur = self._health.get(upstream, {"url": upstream, "healthy": True})
            cur["healthy"] = False
            import time
            cur["lastCheck"] = int(time.time() * 1000)
            cur["error"] = str(e)
            self._health[upstream] = cur

    def destroy(self) -> None:
        for task in self._tasks.values():
            if not task.done():
                task.cancel()
        self._tasks.clear()
