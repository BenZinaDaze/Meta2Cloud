from typing import Optional

from webui.core.app_logging import _log_store
from webui.core.runtime import get_config

def logs_payload(limit: int, category: Optional[str], level: Optional[str]):
    _log_store.set_retention_days(get_config().webui.log_retention_days)
    return {
        "items": _log_store.read(limit=limit, category=category, level=level),
        "summary": _log_store.summary(),
        "maxId": _log_store.max_id(),
    }
