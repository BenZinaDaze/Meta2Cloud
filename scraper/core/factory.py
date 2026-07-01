from typing import Dict, List, Type
from scraper.core.base_spider import BaseSpider
from scraper.models import MediaItem

class SpiderFactory:
    _spiders: Dict[str, BaseSpider] = {}

    @classmethod
    def register(cls, spider: BaseSpider):
        cls._spiders[spider.site_id] = spider

    @classmethod
    def get_spider(cls, site_id: str) -> BaseSpider:
        if site_id not in cls._spiders:
            raise ValueError(f"Spider for site '{site_id}' not found.")
        return cls._spiders[site_id]

    @classmethod
    def get_all_spiders(cls) -> List[BaseSpider]:
        return list(cls._spiders.values())

    @classmethod
    def list_sites(cls) -> List[dict]:
        return [
            {
                "id": spider.site_id,
                "name": spider.site_name,
                "rss_hosts": spider.rss_hosts,
            }
            for spider in cls.get_all_spiders()
        ]

    @classmethod
    def search_all(cls, keyword: str) -> List[MediaItem]:
        results = []
        for spider in cls.get_all_spiders():
            try:
                results.extend(spider.search_media(keyword))
            except Exception as e:
                import logging
                logging.error(f"Error searching {spider.site_id}: {e}")
        return results
