from typing import Any, Dict, Optional

from webui.log_store import LogStore
from webui.core.runtime import _APP_LOG_DIR, _LEGACY_APP_LOG_DB, get_config


_log_store = LogStore(_APP_LOG_DIR, retention_days=7, legacy_path=_LEGACY_APP_LOG_DB)


def app_log(
    category: str,
    event: str,
    message: str,
    *,
    level: str = "INFO",
    details: Optional[Dict[str, Any]] = None,
) -> Dict[str, Any]:
    _log_store.set_retention_days(get_config().webui.log_retention_days)
    return _log_store.write(category=category, event=event, level=level, message=message, details=details)
