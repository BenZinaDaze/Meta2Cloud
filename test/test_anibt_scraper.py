from types import SimpleNamespace

import pytest
from fastapi import HTTPException

from scraper.models import MediaItem
from scraper.strategies.anibt_spider import AniBTSpider
from webui.schemas.subscriptions import SubscriptionTestBody
from webui.services import media_actions, subscriptions


class FakeResponse:
    def __init__(self, *, payload=None, content=b""):
        self._payload = payload
        self.content = content

    def json(self):
        return self._payload


def test_anibt_search_media_builds_group_rss_urls(monkeypatch):
    spider = AniBTSpider()

    def fake_request(url, timeout=15):
        if "/api/bgm/search" in url:
            return FakeResponse(payload={"data": [{"bgmId": 543360, "name": "Test Anime"}]})
        if "/api/anime/groups" in url:
            return FakeResponse(payload={"data": [{"slug": "test-sub", "name": "TestSub"}]})
        raise AssertionError(url)

    monkeypatch.setattr(spider, "_request_with_retry", fake_request)

    results = spider.search_media("test")

    assert [item.name for item in results] == ["Test Anime (全部)", "Test Anime [TestSub]"]
    assert results[0].rss_url == "https://anibt.net/rss/anime.xml?bgmId=543360"
    assert results[1].subgroup_id == "test-sub"
    assert results[1].rss_url == "https://anibt.net/rss/anime.xml?bgmId=543360&groupSlug=test-sub"


def test_anibt_get_episodes_parses_rss_magnet_and_size(monkeypatch):
    spider = AniBTSpider()
    rss = b"""<?xml version="1.0" encoding="UTF-8"?>
<rss version="2.0" xmlns:torrent="http://xmlns.ezrss.it/0.1/">
  <channel>
    <item>
      <title>[TestSub] Test Anime - 01 [1080p]</title>
      <pubDate>Wed, 01 Jul 2026 12:00:00 GMT</pubDate>
      <enclosure url="https://anibt.net/torrents/0123456789abcdef0123456789abcdef01234567.torrent" length="1048576" type="application/x-bittorrent" />
      <torrent:infoHash>abcdefabcdefabcdefabcdefabcdefabcdefabcd</torrent:infoHash>
      <torrent:magneturi>magnet:?xt=urn:btih:abcdefabcdefabcdefabcdefabcdefabcdefabcd&amp;dn=Test&amp;tr=https%3A%2F%2Ftracker.anibt.net%2Fannounce</torrent:magneturi>
    </item>
  </channel>
</rss>"""

    monkeypatch.setattr(spider, "_request_with_retry", lambda url, timeout=15: FakeResponse(content=rss))

    episodes = spider.get_episodes("543360", "test-sub")

    assert len(episodes) == 1
    assert episodes[0].title == "[TestSub] Test Anime - 01 [1080p]"
    assert episodes[0].torrent_url.endswith(".torrent")
    assert episodes[0].magnet_url == (
        "magnet:?xt=urn:btih:abcdefabcdefabcdefabcdefabcdefabcdefabcd"
        "&dn=Test&tr=https%3A%2F%2Ftracker.anibt.net%2Fannounce"
    )
    assert episodes[0].file_size_mb == 1.0
    assert episodes[0].site == "anibt"


def test_subscription_detects_and_parses_anibt_url():
    assert subscriptions._detect_site_from_url("https://anibt.net/rss/anime.xml?bgmId=543360") == "anibt"
    assert subscriptions._parse_rss_url(
        "anibt",
        "https://anibt.net/rss/anime.xml?bgmId=543360&groupSlug=test-sub",
    ) == ("543360", "test-sub")


def test_subscription_rejects_anibt_url_without_bgm_id():
    with pytest.raises(HTTPException) as exc:
        subscriptions._parse_rss_url("anibt", "https://anibt.net/rss/anime.xml?groupSlug=test-sub")
    assert exc.value.status_code == 400


def test_scraper_search_payload_includes_rss_url(monkeypatch):
    fake_result = MediaItem(
        media_id="543360",
        name="Test Anime [TestSub]",
        url="https://anibt.net/anime/543360",
        site="anibt",
        subgroup_id="test-sub",
        subgroup_name="TestSub",
        rss_url="https://anibt.net/rss/anime.xml?bgmId=543360&groupSlug=test-sub",
    )
    fake_factory = SimpleNamespace(search_all=lambda keyword: [fake_result])
    monkeypatch.setattr(media_actions, "SpiderFactory", fake_factory)

    payload = media_actions.scraper_search_media_payload("test")

    source = payload["results"][0]["sources"][0]
    assert source["site"] == "anibt"
    assert source["subgroup_name"] == "TestSub"
    assert source["rss_url"] == "https://anibt.net/rss/anime.xml?bgmId=543360&groupSlug=test-sub"


def test_scraper_sites_payload_exposes_rss_hosts():
    payload = media_actions.scraper_sites_payload()

    sites = {site["id"]: site for site in payload["sites"]}
    assert sites["mikan"]["name"] == "Mikan"
    assert "mikan.tangbai.cc" in sites["mikan"]["rss_hosts"]
    assert sites["anibt"]["name"] == "AniBT"
    assert "anibt.net" in sites["anibt"]["rss_hosts"]


def test_subscription_test_body_allows_empty_media_title():
    body = SubscriptionTestBody(
        media_title="",
        site="anibt",
        rss_url="https://anibt.net/rss/group/nix-raws.xml",
    )

    assert body.media_title == ""
    assert body.site == "anibt"
