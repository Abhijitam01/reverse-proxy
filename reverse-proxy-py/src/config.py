import json
import pathlib
from urllib.parse import urlparse

from .types import ProxyConfig


def load_config(file_path: str) -> ProxyConfig:
    p = pathlib.Path(file_path).resolve()
    if not p.exists():
        raise FileNotFoundError(f"Configuration file not found: {p}")
    content = p.read_text(encoding="utf-8")
    try:
        config = json.loads(content)
    except Exception as e:
        raise ValueError(f"Failed to parse configuration file: {e}") from e
    validate_config(config)
    return config


def validate_config(config) -> None:
    if not isinstance(config, dict):
        raise ValueError("Configuration must be an object")
    if "routes" not in config or not isinstance(config["routes"], list):
        raise ValueError("Configuration must have a 'routes' array")
    if len(config["routes"]) == 0:
        raise ValueError("At least one route must be configured")
    for i, route in enumerate(config["routes"]):
        validate_route(route, i)


def validate_route(route, index: int) -> None:
    if not isinstance(route, dict):
        raise ValueError(f"Route {index} must be an object")
    if "match" not in route or not isinstance(route["match"], dict):
        raise ValueError(f"Route {index}: must have a 'match' object")
    match = route["match"]
    if not match.get("host") and not match.get("pathPrefix"):
        raise ValueError(f"Route {index}: match must have at least 'host' or 'pathPrefix'")
    if "host" in match and not isinstance(match["host"], str):
        raise ValueError(f"Route {index}: match.host must be a string")
    if "pathPrefix" in match and not isinstance(match["pathPrefix"], str):
        raise ValueError(f"Route {index}: match.pathPrefix must be a string")
    if "upstream" not in route or not isinstance(route["upstream"], str):
        raise ValueError(f"Route {index}: must have a 'upstream' string")
    parsed = urlparse(route["upstream"])
    if not parsed.scheme or not parsed.netloc:
        raise ValueError(f"Route {index}: upstream must be a valid URL: {route['upstream']}")
    if "stripPrefix" in route and not isinstance(route["stripPrefix"], bool):
        raise ValueError(f"Route {index}: stripPrefix must be a boolean")
    if "addRequestHeaders" in route and not isinstance(route["addRequestHeaders"], dict):
        raise ValueError(f"Route {index}: addRequestHeaders must be an object")
    if "removeRequestHeaders" in route and not isinstance(route["removeRequestHeaders"], list):
        raise ValueError(f"Route {index}: removeRequestHeaders must be an array")
    if "addResponseHeaders" in route and not isinstance(route["addResponseHeaders"], dict):
        raise ValueError(f"Route {index}: addResponseHeaders must be an object")
    if "removeResponseHeaders" in route and not isinstance(route["removeResponseHeaders"], list):
        raise ValueError(f"Route {index}: removeResponseHeaders must be an array")
    if "healthCheck" in route:
        hc = route["healthCheck"]
        if not isinstance(hc, dict):
            raise ValueError(f"Route {index}: healthCheck must be an object")
        if "path" not in hc or not isinstance(hc["path"], str):
            raise ValueError(f"Route {index}: healthCheck.path must be a string")
        if "intervalMs" in hc and (not isinstance(hc["intervalMs"], (int, float)) or hc["intervalMs"] <= 0):
            raise ValueError(f"Route {index}: healthCheck.intervalMs must be > 0")
