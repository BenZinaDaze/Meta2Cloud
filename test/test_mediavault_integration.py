from __future__ import annotations

import sys
from pathlib import Path
from types import SimpleNamespace

ROOT = Path(__file__).resolve().parents[1]
if str(ROOT) not in sys.path:
    sys.path.insert(0, str(ROOT))

from mediaparser.config import Config
from webui.core.runtime import _validate_main_config_payload
import pytest
from fastapi import HTTPException

from webui.services.mediavault import test_mediavault_connection_payload, trigger_mediavault


def test_mediavault_base_url_uses_scheme_and_host():
    cfg = Config.from_dict(
        {
            "mediavault": {
                "scheme": "https",
                "host": "mediavault.example.com:7811",
            }
        }
    )

    assert cfg.mediavault_base_url == "https://mediavault.example.com:7811"


def test_validate_main_config_payload_normalizes_mediavault_host():
    payload = _validate_main_config_payload(
        {
            "mediavault": {
                "scheme": "HTTPS",
                "host": " https://mediavault.example.com:7811/ ",
                "timeout": 15,
            }
        }
    )

    assert payload["mediavault"]["scheme"] == "https"
    assert payload["mediavault"]["host"] == "mediavault.example.com:7811"
    assert payload["mediavault"]["timeout"] == 15


def test_trigger_mediavault_uses_query_params_and_success_field(monkeypatch):
    cfg = Config.from_dict(
        {
            "mediavault": {
                "enabled": True,
                "scheme": "https",
                "host": "mediavault.example.com",
                "api_key": "secret-key",
                "source_dir": "/watch",
                "timeout": 9,
            }
        }
    )
    captured = {}
    logs = []

    def fake_post(url, params=None, timeout=None):
        captured["url"] = url
        captured["params"] = params
        captured["timeout"] = timeout
        return SimpleNamespace(
            status_code=200,
            json=lambda: {"success": True, "message": "已触发整理", "data": None},
        )

    monkeypatch.setattr("webui.services.mediavault.requests.post", fake_post)
    monkeypatch.setattr("webui.services.mediavault.app_log", lambda *args, **kwargs: logs.append((args, kwargs)))

    ok = trigger_mediavault(cfg, run_id="run-1", ok_count=2)

    assert ok is True
    assert captured["url"] == "https://mediavault.example.com/api/v1/monitor/trigger-organize"
    assert captured["params"] == {"api_key": "secret-key", "source_dir": "/watch"}
    assert captured["timeout"] == 9
    assert any(args[1] == "mediavault_trigger_success" for args, _ in logs)


def test_trigger_mediavault_treats_success_false_as_failure(monkeypatch):
    cfg = Config.from_dict(
        {
            "mediavault": {
                "enabled": True,
                "scheme": "https",
                "host": "mediavault.example.com",
                "api_key": "secret-key",
            }
        }
    )
    logs = []

    monkeypatch.setattr(
        "webui.services.mediavault.requests.post",
        lambda *args, **kwargs: SimpleNamespace(
            status_code=200,
            json=lambda: {"success": False, "message": "未触发", "data": None},
        ),
    )
    monkeypatch.setattr("webui.services.mediavault.app_log", lambda *args, **kwargs: logs.append((args, kwargs)))

    ok = trigger_mediavault(cfg, run_id="run-2", ok_count=1)

    assert ok is False
    assert any(args[1] == "mediavault_trigger_failed" for args, _ in logs)


def test_trigger_mediavault_treats_missing_success_as_failure(monkeypatch):
    cfg = Config.from_dict(
        {
            "mediavault": {
                "enabled": True,
                "scheme": "https",
                "host": "mediavault.example.com",
                "api_key": "secret-key",
            }
        }
    )

    monkeypatch.setattr(
        "webui.services.mediavault.requests.post",
        lambda *args, **kwargs: SimpleNamespace(
            status_code=200,
            json=lambda: {"message": "missing success", "data": None},
        ),
    )

    assert trigger_mediavault(cfg, run_id="run-3", ok_count=1) is False


def test_test_mediavault_connection_accepts_405_method_not_allowed(monkeypatch):
    captured = {}

    def fake_post(url, params=None, timeout=None):
        captured["url"] = url
        captured["params"] = params
        captured["timeout"] = timeout
        return SimpleNamespace(
            status_code=405,
            json=lambda: {"detail": "Method Not Allowed"},
        )

    monkeypatch.setattr("webui.services.mediavault.requests.post", fake_post)

    payload = test_mediavault_connection_payload(
        SimpleNamespace(
            scheme="https",
            host="https://mediavault.example.com",
            api_key="secret-key",
            timeout=8,
        )
    )

    assert payload["ok"] is True
    assert captured["url"] == "https://mediavault.example.com/api/v1"
    assert captured["params"] == {"api_key": "secret-key"}
    assert captured["timeout"] == 8


def test_test_mediavault_connection_rejects_other_responses(monkeypatch):
    monkeypatch.setattr(
        "webui.services.mediavault.requests.post",
        lambda *args, **kwargs: SimpleNamespace(
            status_code=200,
            json=lambda: {"success": True},
        ),
    )

    with pytest.raises(HTTPException):
        test_mediavault_connection_payload(
            SimpleNamespace(
                scheme="https",
                host="mediavault.example.com",
                api_key="secret-key",
                timeout=8,
            )
        )
