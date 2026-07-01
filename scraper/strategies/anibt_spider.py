import re
import time
import xml.etree.ElementTree as ET
from typing import Any, List
from urllib.parse import urlencode, urljoin

import requests

from scraper.core.base_spider import BaseSpider
from scraper.models import MagnetItem, MediaItem


class AniBTSpider(BaseSpider):
    BASE_URL = "https://anibt.net"
    HEADERS = {
        "User-Agent": "Meta2Cloud/0.1 (+https://github.com/)",
        "Accept": "application/json, application/rss+xml, application/xml;q=0.9, */*;q=0.8",
    }
    MAX_RETRIES = 3
    RETRY_DELAY = 2

    def _request_with_retry(self, url: str, timeout: int = 15) -> requests.Response:
        last_error = None
        for attempt in range(self.MAX_RETRIES):
            try:
                res = requests.get(url, headers=self.HEADERS, timeout=timeout)
                res.raise_for_status()
                return res
            except Exception as exc:
                last_error = exc
                if attempt < self.MAX_RETRIES - 1:
                    time.sleep(self.RETRY_DELAY)
        raise last_error

    @property
    def site_id(self) -> str:
        return "anibt"

    @property
    def site_name(self) -> str:
        return "AniBT"

    @property
    def rss_hosts(self) -> list[str]:
        return ["anibt.net"]

    def _api_get(self, path: str, params: dict[str, Any] | None = None) -> Any:
        url = urljoin(self.BASE_URL, path)
        if params:
            url = f"{url}?{urlencode(params)}"
        return self._request_with_retry(url).json()

    def _items_from_payload(self, payload: Any) -> list[dict[str, Any]]:
        if isinstance(payload, list):
            return [item for item in payload if isinstance(item, dict)]
        if not isinstance(payload, dict):
            return []
        for key in ("data", "items", "results", "animes", "groups"):
            value = payload.get(key)
            if isinstance(value, list):
                return [item for item in value if isinstance(item, dict)]
            if isinstance(value, dict):
                nested = self._items_from_payload(value)
                if nested:
                    return nested
        return []

    def _pick_title(self, item: dict[str, Any]) -> str:
        for key in ("nameCn", "name_cn", "name", "titleCn", "title_cn", "title", "displayName"):
            value = item.get(key)
            if value:
                return str(value)
        return ""

    def search_media(self, keyword: str) -> List[MediaItem]:
        search_payload = self._api_get("/api/bgm/search", {"q": keyword, "limit": 5})
        media_items: list[MediaItem] = []
        seen: set[str] = set()

        for candidate in self._items_from_payload(search_payload):
            bgm_id = candidate.get("bgmId") or candidate.get("bangumiId") or candidate.get("id")
            if not bgm_id:
                continue
            bgm_id = str(bgm_id)
            if bgm_id in seen:
                continue
            seen.add(bgm_id)

            title = self._pick_title(candidate) or keyword
            detail_url = f"{self.BASE_URL}/anime/{bgm_id}"
            media_items.append(
                MediaItem(
                    media_id=bgm_id,
                    name=f"{title} (全部)",
                    url=detail_url,
                    cover_image=candidate.get("image") or candidate.get("cover") or candidate.get("poster"),
                    site=self.site_id,
                    rss_url=self.build_rss_url(bgm_id),
                )
            )

            for slug, group_name in self._get_groups(bgm_id):
                media_items.append(
                    MediaItem(
                        media_id=bgm_id,
                        name=f"{title} [{group_name}]",
                        url=detail_url,
                        cover_image=candidate.get("image") or candidate.get("cover") or candidate.get("poster"),
                        site=self.site_id,
                        subgroup_id=slug,
                        subgroup_name=group_name,
                        rss_url=self.build_rss_url(bgm_id, slug),
                    )
                )

        return media_items

    def _get_groups(self, bgm_id: str) -> list[tuple[str, str]]:
        try:
            payload = self._api_get("/api/anime/groups", {"bgmId": bgm_id})
        except Exception:
            return []

        groups: list[tuple[str, str]] = []
        seen: set[str] = set()
        for item in self._items_from_payload(payload):
            group = item.get("group") if isinstance(item.get("group"), dict) else item
            slug = group.get("slug") or group.get("groupSlug")
            name = group.get("name") or group.get("displayName") or slug
            if not slug or not name:
                continue
            slug = str(slug)
            if slug in seen:
                continue
            seen.add(slug)
            groups.append((slug, str(name)))
        return groups

    def build_rss_url(self, media_id: str, subgroup_id: str = None) -> str:
        params = {"bgmId": media_id}
        if subgroup_id:
            params["groupSlug"] = subgroup_id
        return f"{self.BASE_URL}/rss/anime.xml?{urlencode(params)}"

    def get_episodes(self, media_id: str, subgroup_id: str = None) -> List[MagnetItem]:
        rss_url = self.build_rss_url(media_id, subgroup_id)
        res = self._request_with_retry(rss_url)
        try:
            root = ET.fromstring(res.content)
        except ET.ParseError:
            return []

        episodes: list[MagnetItem] = []
        for item in root.findall("./channel/item"):
            title = item.findtext("title") or ""
            publish_time = item.findtext("pubDate") or ""
            link = item.findtext("link") or ""
            torrent_url = None
            magnet_url = None
            infohash = None
            file_size_mb = None

            enclosure = item.find("enclosure")
            if enclosure is not None:
                enclosure_url = enclosure.get("url")
                if enclosure_url:
                    if enclosure_url.startswith("magnet:"):
                        magnet_url = enclosure_url
                    else:
                        torrent_url = enclosure_url
                length_bytes = enclosure.get("length")
                if length_bytes and length_bytes.isdigit():
                    file_size_mb = round(int(length_bytes) / (1024 * 1024), 2)

            for elem in item.iter():
                tag = elem.tag.rsplit("}", 1)[-1].lower()
                text = (elem.text or "").strip()
                if tag in {"magneturi", "magnet", "magnetlink"} and text.startswith("magnet:"):
                    magnet_url = text
                elif tag == "infohash" and text:
                    infohash = text

            if link.startswith("magnet:") and not magnet_url:
                magnet_url = link
            if infohash and not magnet_url:
                magnet_url = f"magnet:?xt=urn:btih:{infohash}"
            if torrent_url and not magnet_url:
                match = re.search(r"/([0-9a-fA-F]{40})\.torrent(?:$|\?)", torrent_url)
                if match:
                    magnet_url = f"magnet:?xt=urn:btih:{match.group(1)}"

            episodes.append(
                MagnetItem(
                    title=title,
                    torrent_url=torrent_url,
                    magnet_url=magnet_url,
                    publish_time=publish_time,
                    file_size_mb=file_size_mb,
                    site=self.site_id,
                )
            )

        return episodes
