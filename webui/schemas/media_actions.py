from typing import Optional

from pydantic import BaseModel


class RefreshItemRequest(BaseModel):
    tmdb_id: int
    media_type: str
    drive_folder_id: str
    title: Optional[str] = None
    year: Optional[str] = None
