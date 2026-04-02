from enum import Enum
from typing import Optional


class MediaType(Enum):
    MOVIE = "电影"
    TV = "电视剧"
    COLLECTION = "系列"
    UNKNOWN = "未知"

    @staticmethod
    def from_agent(key: str) -> Optional["MediaType"]:
        _map = {"movie": MediaType.MOVIE, "tv": MediaType.TV}
        return _map.get(key.strip().lower() if key else "")

    def to_agent(self) -> str:
        return {MediaType.MOVIE: "movie", MediaType.TV: "tv"}.get(self, self.value)
