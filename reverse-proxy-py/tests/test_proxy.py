import asyncio
import pytest
import threading
import http.server
import json
import socket
from httpx import AsyncClient, ASGITransport
from fastapi import FastAPI
import uvicorn
from src.server import build_proxy

# helper to create upstream server that echoes headers
def create_upstream_app():
    from fastapi import FastAPI, Request
    from fastapi.responses import JSONResponse
    app = FastAPI()
    @app.api_route("/{path:path}", methods=["GET","POST","PUT","DELETE","PATCH","OPTIONS","HEAD"])
    async def echo(request: Request, path: str):
        headers = dict(request.headers)
        # collect x-received
        resp_headers = {}
        data = {"path": request.url.path}
        for k, v in headers.items():
            if k.lower() in ("host","content-length"):
                continue
            data[f"x-received-{k.lower()}"] = v
        return JSONResponse(content=data)
    return app

def get_free_port():
    s = socket.socket()
    s.bind(("",0))
    port = s.getsockname()[1]
    s.close()
    return port

@pytest.fixture
async def upstream():
    app = create_upstream_app()
    port = get_free_port()
    url = f"http://127.0.0.1:{port}"
    config = uvicorn.Config(app, host="127.0.0.1", port=port, log_level="error")
    server = uvicorn.Server(config)
    thread = threading.Thread(target=server.run, daemon=True)
    thread.start()
    # wait for startup
    for _ in range(30):
        try:
            import httpx
            httpx.get(url + "/", timeout=1)
            break
        except Exception:
            await asyncio.sleep(0.1)
    yield url
    server.should_exit = True
    await asyncio.sleep(0.2)


@pytest.mark.asyncio
async def test_path_prefix_matching(upstream):
    config = {"routes": [{"match": {"pathPrefix": "/api"}, "upstream": upstream}]}
    proxy = build_proxy(config)
    app = proxy["app"]
    async with AsyncClient(transport=ASGITransport(app=app), base_url="http://test") as c:
        resp = await c.get("/api/users")
        assert resp.status_code == 200
        assert resp.json()["path"] == "/api/users"
    proxy["close"]()

@pytest.mark.asyncio
async def test_strip_prefix(upstream):
    config = {"routes": [{"match": {"pathPrefix": "/api"}, "upstream": upstream, "stripPrefix": True}]}
    proxy = build_proxy(config)
    async with AsyncClient(transport=ASGITransport(app=proxy["app"]), base_url="http://test") as c:
        resp = await c.get("/api/users")
        assert resp.status_code == 200
        assert resp.json()["path"] == "/users"
    proxy["close"]()

@pytest.mark.asyncio
async def test_host_matching(upstream):
    config = {"routes": [{"match": {"host": "app.local"}, "upstream": upstream}]}
    proxy = build_proxy(config)
    async with AsyncClient(transport=ASGITransport(app=proxy["app"]), base_url="http://test") as c:
        resp = await c.get("/test", headers={"Host": "app.local"})
        assert resp.status_code == 200
        assert resp.json()["path"] == "/test"
    proxy["close"]()

@pytest.mark.asyncio
async def test_add_request_headers(upstream):
    config = {"routes": [{"match": {"pathPrefix": "/"}, "upstream": upstream, "addRequestHeaders": {"x-custom-header": "custom-value", "x-another": "value123"}}]}
    proxy = build_proxy(config)
    async with AsyncClient(transport=ASGITransport(app=proxy["app"]), base_url="http://test") as c:
        resp = await c.get("/test")
        assert resp.status_code == 200
        assert resp.json()["x-received-x-custom-header"] == "custom-value"
        assert resp.json()["x-received-x-another"] == "value123"
    proxy["close"]()

@pytest.mark.asyncio
async def test_remove_request_headers(upstream):
    config = {"routes": [{"match": {"pathPrefix": "/"}, "upstream": upstream, "removeRequestHeaders": ["x-secret","authorization"] }]}
    proxy = build_proxy(config)
    async with AsyncClient(transport=ASGITransport(app=proxy["app"]), base_url="http://test") as c:
        resp = await c.get("/test", headers={"x-secret": "secret-value", "authorization": "Bearer token"})
        assert resp.status_code == 200
        assert "x-received-x-secret" not in resp.json()
        assert "x-received-authorization" not in resp.json()
    proxy["close"]()

@pytest.mark.asyncio
async def test_add_response_headers(upstream):
    config = {"routes": [{"match": {"pathPrefix": "/"}, "upstream": upstream, "addResponseHeaders": {"x-proxy-version": "1.0.0","x-custom-response": "added-by-proxy"}}]}
    proxy = build_proxy(config)
    async with AsyncClient(transport=ASGITransport(app=proxy["app"]), base_url="http://test") as c:
        resp = await c.get("/test")
        assert resp.status_code == 200
        assert resp.headers.get("x-proxy-version") == "1.0.0"
        assert resp.headers.get("x-custom-response") == "added-by-proxy"
    proxy["close"]()

@pytest.mark.asyncio
async def test_remove_response_headers(upstream):
    config = {"routes": [{"match": {"pathPrefix": "/"}, "upstream": upstream, "removeResponseHeaders": ["content-type"] }]}
    proxy = build_proxy(config)
    async with AsyncClient(transport=ASGITransport(app=proxy["app"]), base_url="http://test") as c:
        resp = await c.get("/test")
        assert resp.status_code == 200
        assert resp.headers.get("content-type") is None
    proxy["close"]()

@pytest.mark.asyncio
async def test_404_no_route(upstream):
    config = {"routes": [{"match": {"pathPrefix": "/api"}, "upstream": upstream}]}
    proxy = build_proxy(config)
    async with AsyncClient(transport=ASGITransport(app=proxy["app"]), base_url="http://test") as c:
        resp = await c.get("/unknown")
        assert resp.status_code == 404
    proxy["close"]()

@pytest.mark.asyncio
async def test_list_routes(upstream):
    config = {"routes": [{"match": {"pathPrefix": "/api"}, "upstream": upstream},{"match": {"pathPrefix": "/docs"}, "upstream": upstream}]}
    proxy = build_proxy(config)
    async with AsyncClient(transport=ASGITransport(app=proxy["app"]), base_url="http://test") as c:
        resp = await c.get("/_proxy/routes")
        assert resp.status_code == 200
        assert resp.json()["success"] is True
        assert len(resp.json()["data"]) == 2
        assert resp.json()["data"][0]["match"]["pathPrefix"] == "/api"
    proxy["close"]()

@pytest.mark.asyncio
async def test_health_endpoint(upstream):
    config = {"routes": [{"match": {"pathPrefix": "/api"}, "upstream": upstream}]}
    proxy = build_proxy(config)
    async with AsyncClient(transport=ASGITransport(app=proxy["app"]), base_url="http://test") as c:
        resp = await c.get("/_proxy/health")
        assert resp.status_code == 200
        assert resp.json()["success"] is True
        assert resp.json()["data"]["status"] == "ok"
    proxy["close"]()

@pytest.mark.asyncio
async def test_forward_5xx(upstream):
    # create error upstream
    from fastapi import FastAPI
    from fastapi.responses import JSONResponse
    err_app = FastAPI()
    @err_app.api_route("/{path:path}", methods=["GET","POST","PUT","DELETE"])
    async def err(path: str):
        return JSONResponse(status_code=502, content={"error": "Bad Gateway","message":"Upstream error"})
    port = get_free_port()
    url = f"http://127.0.0.1:{port}"
    config2 = uvicorn.Config(err_app, host="127.0.0.1", port=port, log_level="error")
    server2 = uvicorn.Server(config2)
    thread2 = threading.Thread(target=server2.run, daemon=True)
    thread2.start()
    for _ in range(30):
        try:
            import httpx
            httpx.get(url+"/", timeout=1)
            break
        except Exception:
            await asyncio.sleep(0.1)
    cfg = {"routes": [{"match": {"pathPrefix": "/"}, "upstream": url}]}
    proxy = build_proxy(cfg)
    async with AsyncClient(transport=ASGITransport(app=proxy["app"]), base_url="http://test") as c:
        resp = await c.get("/test")
        assert resp.status_code == 502
        assert resp.json()["error"] == "Bad Gateway"
    proxy["close"]()
    server2.should_exit = True
    await asyncio.sleep(0.2)

@pytest.mark.asyncio
async def test_query_string(upstream):
    config = {"routes": [{"match": {"pathPrefix": "/api"}, "upstream": upstream, "stripPrefix": True}]}
    proxy = build_proxy(config)
    async with AsyncClient(transport=ASGITransport(app=proxy["app"]), base_url="http://test") as c:
        resp = await c.get("/api/users?page=2&limit=10")
        assert resp.status_code == 200
        assert resp.json()["path"] == "/users"
    proxy["close"]()

@pytest.mark.asyncio
async def test_multiple_routes(upstream):
    config = {"routes": [{"match": {"pathPrefix": "/api"}, "upstream": upstream, "stripPrefix": True},{"match": {"pathPrefix": "/static"}, "upstream": upstream, "stripPrefix": False}]}
    proxy = build_proxy(config)
    async with AsyncClient(transport=ASGITransport(app=proxy["app"]), base_url="http://test") as c:
        api = await c.get("/api/data")
        assert api.status_code == 200
        assert api.json()["path"] == "/data"
        static = await c.get("/static/file.js")
        assert static.status_code == 200
        assert static.json()["path"] == "/static/file.js"
    proxy["close"]()
