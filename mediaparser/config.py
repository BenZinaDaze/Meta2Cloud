"""
config.py —— 从 YAML 文件加载全局配置，对外提供结构化的配置对象。

用法：
    from mediaparser.config import Config

    cfg = Config()                         # 自动查找 config.yaml
    cfg = Config("/path/to/config.yaml")   # 指定路径
    cfg = Config.from_dict({...})          # 从字典创建（测试用）

    print(cfg.tmdb.api_key)
    print(cfg.parser.custom_words)
"""
from __future__ import annotations

import logging
import os
from dataclasses import dataclass, field
from pathlib import Path
from typing import List, Optional

logger = logging.getLogger(__name__)

# 默认配置文件查找路径（相对于调用方的工作目录 → 项目根）
_DEFAULT_SEARCH_PATHS = [
    Path.cwd() / "config" / "config.yaml",
    Path(__file__).parent.parent / "config" / "config.yaml",
    Path.cwd() / "config.yaml",                           # 向后兼容：根目录
    Path(__file__).parent.parent / "config.yaml",         # 向后兼容：根目录
]


# ─────────────────────────────────────────────────────────
# 配置数据类
# ─────────────────────────────────────────────────────────

@dataclass
class TmdbConfig:
    api_key: str = ""
    language: str = "zh-CN"
    proxy: str = ""
    timeout: int = 10

    @classmethod
    def from_dict(cls, d: dict) -> "TmdbConfig":
        return cls(
            api_key=str(d.get("api_key") or ""),
            language=str(d.get("language") or "zh-CN"),
            proxy=str(d.get("proxy") or ""),
            timeout=int(d.get("timeout") or 10),
        )


@dataclass
class ParserConfig:
    custom_words: List[str] = field(default_factory=list)
    custom_release_groups: List[str] = field(default_factory=list)

    @classmethod
    def from_dict(cls, d: dict) -> "ParserConfig":
        return cls(
            custom_words=list(d.get("custom_words") or []),
            custom_release_groups=list(d.get("custom_release_groups") or []),
        )


@dataclass
class DriveConfig:
    credentials_json: str = "config/credentials.json"
    token_json: str = "config/token.json"
    scan_folder_id: str = ""

    @classmethod
    def from_dict(cls, d: dict) -> "DriveConfig":
        return cls(
            credentials_json=str(d.get("credentials_json") or "config/credentials.json"),
            token_json=str(d.get("token_json") or "config/token.json"),
            scan_folder_id=str(d.get("scan_folder_id") or ""),
        )


@dataclass
class OrganizerConfig:
    root_folder_id: str = ""
    movie_root_id: str = ""
    tv_root_id: str = ""

    @classmethod
    def from_dict(cls, d: dict) -> "OrganizerConfig":
        return cls(
            root_folder_id=str(d.get("root_folder_id") or ""),
            movie_root_id=str(d.get("movie_root_id") or ""),
            tv_root_id=str(d.get("tv_root_id") or ""),
        )


@dataclass
class PipelineConfig:
    skip_tmdb: bool = False
    move_on_tmdb_miss: bool = False
    dry_run: bool = False

    @classmethod
    def from_dict(cls, d: dict) -> "PipelineConfig":
        return cls(
            skip_tmdb=bool(d.get("skip_tmdb", False)),
            move_on_tmdb_miss=bool(d.get("move_on_tmdb_miss", False)),
            dry_run=bool(d.get("dry_run", False)),
        )


@dataclass
class TelegramConfig:
    bot_token: str = ""
    chat_id: str = ""
    debounce_seconds: int = 0    # 防抖延时（秒），0 = 立即触发

    @classmethod
    def from_dict(cls, d: dict) -> "TelegramConfig":
        return cls(
            bot_token=str(d.get("bot_token") or ""),
            chat_id=str(d.get("chat_id") or ""),
            debounce_seconds=int(d.get("debounce_seconds") or 0),
        )


@dataclass
class WebUIConfig:
    username: str = "admin"
    password: str = ""           # 明文密码（配置文件私有，用 hmac.compare_digest 比较）
    secret_key: str = ""         # JWT 签名密钥，空则自动生成并持久化到 config/data/.jwt_secret
    token_expire_hours: int = 24
    webhook_secret: str = ""     # /trigger 端点 webhook 密钥；空则不校验（仅内网使用时）
    log_retention_days: int = 7  # 日志保留天数，按天滚动存储

    @classmethod
    def from_dict(cls, d: dict) -> "WebUIConfig":
        return cls(
            username=str(d.get("username") or "admin"),
            password=str(d.get("password") or ""),
            secret_key=str(d.get("secret_key") or ""),
            token_expire_hours=int(d.get("token_expire_hours") or 24),
            webhook_secret=str(d.get("webhook_secret") or ""),
            log_retention_days=max(1, int(d.get("log_retention_days") or 7)),
        )


@dataclass
class Aria2Config:
    host: str = "127.0.0.1"
    port: int = 6800
    path: str = "/jsonrpc"
    secret: str = ""
    secure: bool = False

    @classmethod
    def from_dict(cls, d: dict) -> "Aria2Config":
        return cls(
            host=str(d.get("host") or "127.0.0.1"),
            port=int(d.get("port") or 6800),
            path=str(d.get("path") or "/jsonrpc"),
            secret=str(d.get("secret") or ""),
            secure=bool(d.get("secure", False)),
        )


@dataclass
class Config:
    tmdb: TmdbConfig = field(default_factory=TmdbConfig)
    parser: ParserConfig = field(default_factory=ParserConfig)
    drive: DriveConfig = field(default_factory=DriveConfig)
    organizer: OrganizerConfig = field(default_factory=OrganizerConfig)
    pipeline: PipelineConfig = field(default_factory=PipelineConfig)
    telegram: TelegramConfig = field(default_factory=TelegramConfig)
    webui: WebUIConfig = field(default_factory=WebUIConfig)
    aria2: Aria2Config = field(default_factory=Aria2Config)


    # ── 加载方法 ─────────────────────────────────────────

    @classmethod
    def load(cls, path: Optional[str | Path] = None) -> "Config":
        """
        从 YAML 文件加载配置。
        path 为 None 时，自动按 _DEFAULT_SEARCH_PATHS 顺序查找。
        """
        try:
            import yaml  # 懒加载，避免没装 pyyaml 时整个模块崩溃
        except ImportError:
            logger.warning("未安装 pyyaml，将使用默认配置。运行 pip install pyyaml 来启用配置文件支持。")
            return cls()

        config_path = cls._find_config(path)
        if config_path is None:
            logger.warning("未找到 config.yaml，将使用默认配置。")
            return cls()

        try:
            with open(config_path, "r", encoding="utf-8") as f:
                data = yaml.safe_load(f) or {}
            logger.info("已加载配置文件：%s", config_path)
            return cls.from_dict(data)
        except Exception as e:
            logger.error("读取配置文件失败：%s，将使用默认配置。", e)
            return cls()

    @classmethod
    def from_dict(cls, d: dict) -> "Config":
        """从字典创建配置（便于测试）"""
        return cls(
            tmdb=TmdbConfig.from_dict(d.get("tmdb") or {}),
            parser=ParserConfig.from_dict(d.get("parser") or {}),
            drive=DriveConfig.from_dict(d.get("drive") or {}),
            organizer=OrganizerConfig.from_dict(d.get("organizer") or {}),
            pipeline=PipelineConfig.from_dict(d.get("pipeline") or {}),
            telegram=TelegramConfig.from_dict(d.get("telegram") or {}),
            webui=WebUIConfig.from_dict(d.get("webui") or {}),
            aria2=Aria2Config.from_dict(d.get("aria2") or {}),
        )

    @staticmethod
    def _find_config(path: Optional[str | Path]) -> Optional[Path]:
        if path:
            p = Path(path)
            return p if p.exists() else None
        for candidate in _DEFAULT_SEARCH_PATHS:
            if candidate.exists():
                return candidate
        return None

    # ── 便利属性 ─────────────────────────────────────────

    @property
    def tmdb_proxy(self) -> Optional[str]:
        return self.tmdb.proxy or None

    def is_tmdb_ready(self) -> bool:
        """是否具备 TMDB 查询条件（有 api_key）"""
        return bool(self.tmdb.api_key)
