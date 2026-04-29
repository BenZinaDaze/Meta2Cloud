import asyncio
import json
import logging
from typing import Any, Dict, List

from fastapi import APIRouter, WebSocket, WebSocketDisconnect
from webui.core.auth import verify_token

logger = logging.getLogger(__name__)

router = APIRouter()


class _LogBroadcaster:
    """Manages WebSocket connections and broadcasts log entries to connected clients."""

    def __init__(self):
        self._connections: List[WebSocket] = []
        self._lock = asyncio.Lock()

    async def connect(self, ws: WebSocket) -> None:
        async with self._lock:
            self._connections.append(ws)

    async def disconnect(self, ws: WebSocket) -> None:
        async with self._lock:
            if ws in self._connections:
                self._connections.remove(ws)

    async def broadcast(self, data: Dict[str, Any]) -> None:
        async with self._lock:
            stale: List[WebSocket] = []
            for ws in self._connections:
                try:
                    await ws.send_text(json.dumps(data, ensure_ascii=False))
                except Exception:
                    stale.append(ws)
            for ws in stale:
                self._connections.remove(ws)

    def broadcast_sync(self, data: Dict[str, Any]) -> None:
        """Thread-safe broadcast from non-async context (e.g., pipeline thread)."""
        asyncio.run_coroutine_threadsafe(self.broadcast(data), _get_loop())

    @property
    def connected_count(self) -> int:
        return len(self._connections)


_broadcaster = _LogBroadcaster()
_loop: asyncio.AbstractEventLoop | None = None


async def init_loop() -> None:
    """Must be called from async context during app startup to capture the running event loop."""
    global _loop
    _loop = asyncio.get_running_loop()


def _get_loop() -> asyncio.AbstractEventLoop:
    if _loop is None or _loop.is_closed():
        raise RuntimeError("Event loop not initialized — call init_loop() at app startup")
    return _loop


def get_broadcaster() -> _LogBroadcaster:
    return _broadcaster


@router.websocket("/api/logs/live")
async def logs_websocket(ws: WebSocket):
    token = ws.query_params.get("token", "")
    if not verify_token(token):
        await ws.close(code=4001)
        return

    await ws.accept()
    b = get_broadcaster()
    await b.connect(ws)
    try:
        while True:
            await ws.receive_text()
    except WebSocketDisconnect:
        pass
    except Exception:
        pass
    finally:
        await b.disconnect(ws)
