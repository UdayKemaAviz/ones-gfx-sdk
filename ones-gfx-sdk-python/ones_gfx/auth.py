from __future__ import annotations

import base64
import json
import logging
import threading
import time
from abc import ABC, abstractmethod
from typing import Callable

import requests

from .exceptions import AuthenticationError, TransportError

logger = logging.getLogger(__name__)


class AuthBase(ABC):
    @abstractmethod
    def apply(self, headers: dict) -> None:
        pass

    @abstractmethod
    def refresh(self) -> None:
        pass

    def needs_proactive_refresh(self) -> bool:
        return False

    def close(self) -> None:
        return None


class JWTAuth(AuthBase):
    def __init__(
        self,
        access_token: str,
        refresh_token: str,
        refresh_url: str,
        on_token_refresh: Callable[[str, str, int], None] | None = None,
        verify_tls: bool | str = True,
        proactive_refresh_buffer_s: int = 10,
        refresh_timeout: int = 10,
    ):
        if not access_token or not refresh_token:
            raise ValueError("Both access_token and refresh_token are required.")
        if not refresh_url:
            raise ValueError("refresh_url is required.")
        self._access_token = access_token
        self._refresh_token = refresh_token
        self._refresh_url = refresh_url
        self._on_token_refresh = on_token_refresh
        self._verify_tls = verify_tls
        self._buffer_s = proactive_refresh_buffer_s
        self._refresh_timeout = refresh_timeout
        self._access_exp: float | None = self._extract_exp(access_token)
        self._lock = threading.Lock()

    def apply(self, headers: dict) -> None:
        headers["Authorization"] = f"Bearer {self._access_token}"

    def needs_proactive_refresh(self) -> bool:
        if self._access_exp is None:
            return False
        return time.time() >= (self._access_exp - self._buffer_s)

    def refresh(self) -> None:
        with self._lock:
            if not self.needs_proactive_refresh() and self._access_exp is not None:
                return
            self._do_refresh()

    def _do_refresh(self) -> None:
        try:
            response = requests.post(
                self._refresh_url,
                json={"refresh_token": self._refresh_token},
                headers={"Content-Type": "application/json"},
                timeout=self._refresh_timeout,
                verify=self._verify_tls,
            )
        except requests.exceptions.RequestException as e:
            raise TransportError(f"Failed to reach refresh endpoint: {e}") from e
        if response.status_code != 200:
            raise AuthenticationError(
                f"Token refresh failed: {self._extract_error_detail(response)}",
                status_code=response.status_code,
                response_body=self._safe_json(response),
            )
        payload = self._safe_json(response)
        if not isinstance(payload, dict):
            raise AuthenticationError("Refresh endpoint returned non-JSON response.")
        new_access = payload.get("access_token")
        new_refresh = payload.get("refresh_token")
        expires_in = payload.get("expires_in", 0)
        if not new_access or not new_refresh:
            raise AuthenticationError("Refresh response missing access_token or refresh_token.")
        self._access_token = new_access
        self._refresh_token = new_refresh
        self._access_exp = self._extract_exp(new_access)
        if self._access_exp is None and expires_in:
            self._access_exp = time.time() + float(expires_in)
        if self._on_token_refresh is not None:
            self._on_token_refresh(new_access, new_refresh, expires_in)

    @staticmethod
    def _extract_exp(token: str) -> float | None:
        try:
            parts = token.split(".")
            if len(parts) != 3:
                return None
            payload_segment = parts[1]
            padding = "=" * (-len(payload_segment) % 4)
            decoded = base64.urlsafe_b64decode(payload_segment + padding)
            claims = json.loads(decoded)
            exp = claims.get("exp")
            return float(exp) if exp is not None else None
        except (ValueError, json.JSONDecodeError, TypeError):
            return None

    @staticmethod
    def _extract_error_detail(response) -> str:
        try:
            body = response.json()
            if isinstance(body, dict):
                return body.get("error") or body.get("message") or str(body)
            return str(body)
        except ValueError:
            return response.text[:200] if response.text else f"HTTP {response.status_code}"

    @staticmethod
    def _safe_json(response):
        try:
            return response.json()
        except ValueError:
            return response.text
