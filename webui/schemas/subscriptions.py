from typing import Optional

from pydantic import BaseModel, Field


class SubscriptionBaseBody(BaseModel):
    name: str = Field(..., min_length=1)
    media_title: str = Field(..., min_length=1)
    media_type: str = Field(default="tv")
    tmdb_id: Optional[int] = None
    poster_url: Optional[str] = None
    site: str = Field(..., min_length=1)
    rss_url: str = Field(..., min_length=1)
    subgroup_name: Optional[str] = ""
    season_number: int = Field(default=1, ge=1, le=99)
    start_episode: int = Field(default=1, ge=1, le=9999)
    keyword_all: list[str] = Field(default_factory=list)
    push_target: str = Field(..., pattern="^(aria2|u115)$")
    enabled: bool = True


class SubscriptionCreateBody(SubscriptionBaseBody):
    pass


class SubscriptionUpdateBody(SubscriptionBaseBody):
    pass


class SubscriptionTestBody(BaseModel):
    media_title: str = Field(..., min_length=1)
    poster_url: Optional[str] = None
    site: str = Field(..., min_length=1)
    rss_url: str = Field(..., min_length=1)
    season_number: int = Field(default=1, ge=1, le=99)
    start_episode: int = Field(default=1, ge=1, le=9999)
    keyword_all: list[str] = Field(default_factory=list)
