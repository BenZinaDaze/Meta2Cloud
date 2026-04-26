from typing import List, Optional

from pydantic import BaseModel


class IngestRecord(BaseModel):
    id: int
    media_type: str
    tmdb_id: int
    title: str
    original_title: str
    year: str
    season: Optional[int] = None
    episode: Optional[int] = None
    episode_title: str
    poster_path: str
    poster_url: Optional[str] = None
    drive_folder_id: str
    original_name: str
    status: str
    error_message: str
    ingested_at: str


class IngestPagination(BaseModel):
    page: int
    page_size: int
    total: int
    total_pages: int


class IngestHistoryResponse(BaseModel):
    items: List[IngestRecord]
    pagination: IngestPagination


class IngestStatsResponse(BaseModel):
    days: int
    total: int
    movies: int
    tv_episodes: int
    success: int
    failed: int
    no_tmdb: int
