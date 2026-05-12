from typing import Any, Dict, Optional

from pydantic import BaseModel


class ConfigSaveBody(BaseModel):
    data: Dict[str, Any]


class MediaVaultTestBody(BaseModel):
    enabled: Optional[bool] = None
    scheme: Optional[str] = None
    host: Optional[str] = None
    api_key: Optional[str] = None
    timeout: Optional[int] = None


class ParserTestBody(BaseModel):
    filename: str


class U115CreateSessionBody(BaseModel):
    client_id: Optional[str] = None
    token_json: Optional[str] = None


class U115ExchangeBody(BaseModel):
    client_id: Optional[str] = None
    token_json: Optional[str] = None


class U115OfflineAddUrlsBody(BaseModel):
    urls: str
    wp_path_id: Optional[str] = None


class U115OfflineDeleteBody(BaseModel):
    info_hashes: list[str]
    del_source_file: int = 0


class U115OfflineClearBody(BaseModel):
    flag: int = 0
