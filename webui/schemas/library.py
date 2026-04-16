from typing import List, Optional

from pydantic import BaseModel


class EpisodeStatus(BaseModel):
    episode_number: int
    episode_title: str
    air_date: str
    in_library: bool


class SeasonStatus(BaseModel):
    season_number: int
    season_name: str
    poster_url: Optional[str]
    episode_count: int
    in_library_count: int
    episodes: List[EpisodeStatus]


class MediaItem(BaseModel):
    tmdb_id: int
    title: str
    original_title: str
    year: str
    media_type: str
    poster_url: Optional[str]
    backdrop_url: Optional[str]
    overview: str
    rating: float
    seasons: Optional[List[SeasonStatus]] = None
    total_episodes: Optional[int] = None
    in_library_episodes: Optional[int] = None
    status: Optional[str] = None
    drive_folder_id: Optional[str] = None


class LibraryResponse(BaseModel):
    movies: List[MediaItem]
    tv_shows: List[MediaItem]
    total_movies: int
    total_tv: int
    scanned_at: Optional[str] = None
    hint: Optional[str] = None


class StatsResponse(BaseModel):
    total_movies: int
    total_tv_shows: int
    total_episodes_in_library: int
    total_episodes_on_tmdb: int
    completion_rate: float
