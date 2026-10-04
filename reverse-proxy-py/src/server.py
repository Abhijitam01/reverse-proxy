"""FastAPI reverse proxy factory."""

from __future__ import annotations

import urllib.parse

import httpx
from fastapi import FastAPI, Request
from fastapi.responses import JSONResponse, Response

from .health import HealthChecker
from .router import match_route, strip_path_prefix

HOP_BY_HOP = {"connection","keep-alive","transfer-encoding","te","trailer","upgrade","proxy-authorization","proxy-authenticate","content-length"}


def _copy_headers(source: dict) -> dict:
    dest = {}
    for k, v in source.items():
        lk = k.lower()
        if lk in HOP_BY_HOP:
            continue
        dest[lk] = v
    return dest


def _apply_mods(headers: dict, add: dict | None, remove: list[str] | None) -> dict:
    mod = dict(headers)
    if remove:
        for h in remove:
            mod.pop(h.lower(), None)
    if add:
        for k, v in add.items():
            mod[k.lower()] = v
    return mod


def build_app(config: dict, health_checker: HealthChecker | None = None) -> FastAPI:
    """Compatibility wrapper matching TS buildProxy(config). Returns FastAPI app."""
    result = build_proxy(config)
    return result["app"]


def build_proxy(config: dict, options: dict | None = None):
    health_checker = HealthChecker()
    current_routes: list[dict] = list(config.get("routes", []))
    for r in current_routes:
        health_checker.register_route(r)

    app = FastAPI(title="reverse-proxy-py")

    # management endpoints must be defined before catch-all
    @app.get("/_proxy/routes")
    async def list_routes():
        infos = []
        for idx, route in enumerate(current_routes):
            infos.append({**route, "index": idx, "upstreamHealth": health_checker.get_health(route["upstream"])})
        return {"success": True, "data": infos}

    @app.post("/_proxy/routes")
    async def add_route(request: Request):
        try:
            body = await request.json()
        except Exception as e:
            return JSONResponse(status_code=400, content={"success": False, "error": str(e)})
        if not body.get("match") or not body.get("upstream"):
            return JSONResponse(status_code=400, content={"success": False, "error": "Invalid route: must have 'match' and 'upstream'"})
        if not body["match"].get("host") and not body["match"].get("pathPrefix"):
            return JSONResponse(status_code=400, content={"success": False, "error": "Invalid route: must have 'host' or 'pathPrefix' in match"})
        current_routes.append(body)
        health_checker.register_route(body)
        return JSONResponse(status_code=201, content={"success": True, "data": {"route": body, "index": len(current_routes)-1}})

    @app.delete("/_proxy/routes/{index}")
    async def delete_route(index: int):
        if index < 0 or index >= len(current_routes):
            return JSONResponse(status_code=404, content={"success": False, "error": "Route not found"})
        removed = current_routes[index]
        health_checker.unregister_route(removed)
        current_routes.pop(index)
        return {"success": True, "message": "Route removed", "data": removed}

    @app.get("/_proxy/health")
    async def proxy_health():
        all_h = health_checker.get_all_health()
        healthy_count = sum(1 for h in all_h.values() if h.get("healthy"))
        unhealthy = [{"url": h["url"], "error": h.get("error")} for h in all_h.values() if not h.get("healthy")]
        return {"success": True, "data": {"status": "ok", "upstreamCount": len(all_h), "healthyCount": healthy_count, "unhealthyUpstreams": unhealthy}}

    @app.api_route("/{full_path:path}", methods=["GET","POST","PUT","DELETE","PATCH","OPTIONS","HEAD"])
    async def proxy_handler(full_path: str, request: Request):
        # reconstruct host and path
        host = request.headers.get("host")
        # request.url.path includes leading /
        path = request.url.path
        # match route
        matched = match_route(current_routes, host, path)
        if not matched:
            return JSONResponse(status_code=404, content={"success": False, "error": "No matching route found"})
        route, matched_prefix = matched
        # health check
        if not health_checker.is_healthy(route["upstream"]):
            return JSONResponse(status_code=502, content={"success": False, "error": "Upstream server is unhealthy"})
        # construct forward path
        original_url = str(request.url)
        # we need path + query
        parsed_req = urllib.parse.urlparse(original_url)
        path_only = parsed_req.path
        query = parsed_req.query
        if route.get("stripPrefix") and matched_prefix:
            final_path = strip_path_prefix(path_only, matched_prefix)
        else:
            final_path = path_only
        if query:
            final_path = f"{final_path}?{query}"
        # headers
        forward_headers = _copy_headers(dict(request.headers))
        forward_headers = _apply_mods(forward_headers, route.get("addRequestHeaders"), route.get("removeRequestHeaders"))
        # host header for upstream
        upstream_parsed = urllib.parse.urlparse(route["upstream"])
        forward_headers["host"] = upstream_parsed.netloc
        # body
        body = await request.body()
        upstream_url = urllib.parse.urljoin(route["upstream"].rstrip("/") + "/", final_path.lstrip("/")) if not final_path.startswith("/") else urllib.parse.urljoin(route["upstream"], final_path)
        # alternative: use urljoin correctly
        if final_path.startswith("/"):
            upstream_url = route["upstream"].rstrip("/") + final_path
        else:
            upstream_url = route["upstream"].rstrip("/") + "/" + final_path

        try:
            async with httpx.AsyncClient(follow_redirects=False, timeout=30) as client:
                upstream_resp = await client.request(
                    method=request.method,
                    url=upstream_url,
                    headers=forward_headers,
                    content=body,
                )
        except httpx.TimeoutException:
            return JSONResponse(status_code=504, content={"success": False, "error": "Gateway Timeout", "message": "Upstream server did not respond in time"})
        except Exception as e:
            return JSONResponse(status_code=502, content={"success": False, "error": "Bad Gateway", "message": "Failed to reach upstream server", "details": str(e)})

        # copy response headers
        resp_headers = _copy_headers(dict(upstream_resp.headers))
        resp_headers = _apply_mods(resp_headers, route.get("addResponseHeaders"), route.get("removeResponseHeaders"))
        # build response without auto media_type to respect header removal
        # Starlette Response with media_type would re-add content-type even if removed, so handle explicitly
        if "content-type" not in resp_headers:
            # return raw Response without media_type to avoid auto-adding content-type
            from starlette.responses import Response as StarletteResponse

            # Use raw_headers to bypass charset logic
            resp = StarletteResponse(content=upstream_resp.content, status_code=upstream_resp.status_code, headers=resp_headers)
            # remove auto-added content-type if present
            if "content-type" in resp.headers:
                # starlette adds default; ensure removal by deleting from raw headers
                del resp.headers["content-type"]
            return resp
        return Response(content=upstream_resp.content, status_code=upstream_resp.status_code, headers=resp_headers, media_type=resp_headers.get("content-type"))

    # expose for tests
    app.state.health_checker = health_checker
    app.state.get_routes = lambda: list(current_routes)

    def close():
        health_checker.destroy()

    return {"app": app, "healthChecker": health_checker, "close": close, "get_routes": lambda: list(current_routes)}


def main():
    import sys
    import uvicorn

    from .config import load_config

    config_path = sys.argv[1] if len(sys.argv) > 1 else "config.json"
    config = load_config(config_path)
    proxy = build_proxy(config)
    uvicorn.run(proxy["app"], host="0.0.0.0", port=3000)


if __name__ == "__main__":
    main()
