from abc import ABC, abstractmethod
from typing import List
from scraper.models import MediaItem, MagnetItem

class BaseSpider(ABC):
    @property
    @abstractmethod
    def site_id(self) -> str:
        """
        Return the unique identifier for the site (e.g. 'mikan')
        """
        pass

    @property
    def site_name(self) -> str:
        """
        Human-readable site name for UI display.
        """
        return self.site_id

    @property
    def rss_hosts(self) -> List[str]:
        """
        Hostname suffixes that should be recognized as this site's RSS URLs.
        """
        return []

    @abstractmethod
    def search_media(self, keyword: str) -> List[MediaItem]:
        """
        Search for MediaItems by keyword.
        Returns a list of MediaItems matching the keyword.
        """
        pass

    @abstractmethod
    def get_episodes(self, media_id: str, subgroup_id: str = None) -> List[MagnetItem]:
        """
        Fetch magnet items for a specific Media ID and optional subgroup ID.
        """
        pass
