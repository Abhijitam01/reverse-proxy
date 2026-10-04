"""Request routing logic."""

from __future__ import annotations

from .types import RouteConfig


def match_route(routes: list[RouteConfig], host: str | None, path: str):
    """Find first matching route. Returns (route, matchedPrefix) or None."""
    for route in routes:
        match = route.get("match", {})
        # host check
        if "host" in match and match["host"]:
            if not host or not host_matches(host, match["host"]):
                continue
        # pathPrefix check
        if "pathPrefix" in match and match["pathPrefix"]:
            if not path_matches(path, match["pathPrefix"]):
                continue
            return route, match["pathPrefix"]
        if "host" in match and match["host"]:
            return route, None
    return None


def host_matches(actual: str, pattern: str) -> bool:
    actual_host = actual.split(":")[0]
    if pattern == actual_host:
        return True
    if pattern.startswith("*."):
        suffix = pattern[1:]  # ".example.com"
        return actual_host.endswith(suffix)
    return False


def path_matches(path: str, prefix: str) -> bool:
    if prefix == "/":
        return True
    normalized = prefix if prefix.startswith("/") else "/" + prefix
    return path == normalized or path.startswith(normalized + "/")


def strip_path_prefix(path: str, prefix: str) -> str:
    if prefix == "/":
        return path
    normalized = prefix if prefix.startswith("/") else "/" + prefix
    if path == normalized:
        return "/"
    if path.startswith(normalized + "/"):
        return path[len(normalized):]
    return path
