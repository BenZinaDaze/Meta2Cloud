from fastapi import APIRouter

from webui.schemas.config import ConfigSaveBody, ParserTestBody
from webui.services.config import (
    drive_oauth_status_payload,
    drive_test_connection_payload,
    parser_test_payload,
    read_config_payload,
    read_main_config_payload,
    read_parser_rules_payload,
    write_config_payload,
    write_main_config_payload,
    write_parser_rules_payload,
)

router = APIRouter()


@router.get("/api/config")
async def read_config():
    return read_config_payload()


@router.get("/api/config/main")
async def read_main_config():
    return read_main_config_payload()


@router.get("/api/config/parser-rules")
async def read_parser_rules_config():
    return read_parser_rules_payload()


@router.post("/api/parser/test")
async def parser_test(body: ParserTestBody):
    return parser_test_payload(body.filename)


@router.put("/api/config")
async def write_config(body: ConfigSaveBody):
    return write_config_payload(body.data)


@router.put("/api/config/main")
async def write_main_config(body: ConfigSaveBody):
    return write_main_config_payload(body.data)


@router.put("/api/config/parser-rules")
async def write_parser_rules_config(body: ConfigSaveBody):
    return write_parser_rules_payload(body.data)


@router.get("/api/drive/oauth/status")
async def drive_oauth_status():
    return drive_oauth_status_payload()


@router.post("/api/drive/test")
async def drive_test_connection():
    return drive_test_connection_payload()
