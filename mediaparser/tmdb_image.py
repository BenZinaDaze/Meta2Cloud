from __future__ import annotations

import re
from typing import Optional
from urllib.parse import urlparse


DEFAULT_TMDB_IMAGE_BASE_URL = "https://image.tmdb.org"


def normalize_tmdb_image_base_url(base_url: Optional[str]) -> str:
    value = (base_url or "").strip().rstrip("/")
    return value or DEFAULT_TMDB_IMAGE_BASE_URL


def build_tmdb_image_url(
    path: Optional[str],
    *,
    size: str = "original",
    base_url: Optional[str] = None,
) -> Optional[str]:
    if not path:
        return None
    normalized_path = path if path.startswith("/") else f"/{path}"
    return f"{normalize_tmdb_image_base_url(base_url)}/t/p/{size}{normalized_path}"


def extract_tmdb_image_path(value: Optional[str]) -> str:
    raw = (value or "").strip()
    if not raw:
        return ""
    if raw.startswith("/"):
        return raw
    parsed = urlparse(raw)
    candidate = parsed.path or raw
    match = re.search(r"/t/p/[^/]+(?P<image_path>/.*)$", candidate)
    if match:
        return match.group("image_path")
    return raw if raw.startswith("/") else ""
