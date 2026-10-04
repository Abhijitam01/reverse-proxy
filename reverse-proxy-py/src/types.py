from __future__ import annotations

from typing import TypedDict


class RouteMatch(TypedDict, total=False):
    host: str
    pathPrefix: str


class HealthCheckConfig(TypedDict, total=False):
    path: str
    intervalMs: int


class RouteConfig(TypedDict, total=False):
    match: RouteMatch
    upstream: str
    stripPrefix: bool
    addRequestHeaders: dict[str, str]
    removeRequestHeaders: list[str]
    addResponseHeaders: dict[str, str]
    removeResponseHeaders: list[str]
    healthCheck: HealthCheckConfig


class ProxyConfig(TypedDict):
    routes: list[RouteConfig]
