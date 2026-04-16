import requests
from webui.core.runtime import logger


def send_telegram(token: str, chat_id: str, text: str) -> None:
    if not token or not chat_id:
        return
    try:
        requests.post(
            f"https://api.telegram.org/bot{token}/sendMessage",
            json={"chat_id": chat_id, "text": text, "parse_mode": "HTML"},
            timeout=10,
        )
    except Exception:
        logger.warning("Telegram 通知发送失败")
