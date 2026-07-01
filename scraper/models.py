from typing import Optional
from pydantic import BaseModel, Field

class MediaItem(BaseModel):
    media_id: str = Field(..., description="The unique ID of the media on the target site")
    name: str = Field(..., description="Media Name/Title")
    url: str = Field(..., description="Link to the media detail page")
    cover_image: Optional[str] = Field(None, description="URL of the cover poster")
    site: str = Field(..., description="The site source, e.g. mikan")
    subgroup_id: Optional[str] = Field(None, description="Subtitle group ID if available")
    subgroup_name: Optional[str] = Field(None, description="Subtitle group name if available")
    rss_url: Optional[str] = Field(None, description="RSS URL for this media/subgroup if available")

class MagnetItem(BaseModel):
    title: str = Field(..., description="Full raw torrent/magnet title")
    torrent_url: Optional[str] = Field(None, description="Direct .torrent download url")
    magnet_url: Optional[str] = Field(None, description="Magnet link")
    publish_time: Optional[str] = Field(None, description="Publish time string")
    file_size_mb: Optional[float] = Field(None, description="File size in MB")
    site: str = Field(..., description="The site source, e.g. mikan")
