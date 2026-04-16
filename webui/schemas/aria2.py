from typing import Dict, Optional

from pydantic import BaseModel


class Aria2AddUriBody(BaseModel):
    uris: list[str]
    options: Optional[Dict[str, str]] = None
    position: Optional[int] = None
    title: Optional[str] = None


class Aria2AddTorrentBody(BaseModel):
    torrent: str
    uris: Optional[list[str]] = None
    options: Optional[Dict[str, str]] = None
    position: Optional[int] = None
    title: Optional[str] = None


class Aria2BatchActionBody(BaseModel):
    gids: list[str]
