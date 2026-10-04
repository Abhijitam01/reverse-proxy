"""Request forwarding via httpx."""

from __future__ import annotations

import urllib.parse

import httpx

HOP_BY_HOP = {"connection","keep-alive","transfer-encoding","te","trailer","upgrade","proxy-authorization","proxy-authenticate","content-length"}


def copy_headers(source: dict, dest: dict) -> None:
    for k, v in source.items():
        lk = k.lower()
        if lk in HOP_BY_HOP:
            continue
        if isinstance(v, str):
            dest[lk] = v
        elif isinstance(v, list):
            dest[lk] = ", ".join(v)
        else:
            dest[lk] = str(v)


def apply_header_mods(headers: dict, add: dict | None, remove: list[str] | None) -> dict:
    mod = dict(headers)
    if remove:
        for h in remove:
            mod.pop(h.lower(), None)
    if add:
        for k, v in add.items():
            mod[k.lower()] = v
    return mod


async def forward_request(scope, receive, send, route: dict, forward_path: str):
    """Forward ASGI request to upstream using httpx. Returns status code and response handling via send."""
    # This is used via FastAPI endpoint impl instead; placeholder for direct ASGI forwarding
    pass
