"""Signed demo-chat capabilities and conservative root-turn capture parsing."""

from __future__ import annotations

import base64
import hmac
import json
import secrets
from collections.abc import Callable, Mapping
from dataclasses import dataclass
from datetime import UTC, datetime, timedelta
from typing import cast

COOKIE_NAME = "assay_demo_chat"
COOKIE_LIFETIME = timedelta(days=30)


@dataclass(frozen=True, slots=True)
class ChatIdentity:
    session_id: str
    browser_id: str


class SessionSigner:
    """Issue bounded, authenticated bearer capabilities for one demo browser."""

    def __init__(self, secret: bytes, *, now: Callable[[], datetime] | None = None) -> None:
        if len(secret) < 32:
            raise ValueError("Chat signing secret must be at least 32 bytes")
        self._secret = secret
        self._now = now or (lambda: datetime.now(UTC))

    def create(self) -> ChatIdentity:
        return ChatIdentity(session_id=secrets.token_hex(16), browser_id=secrets.token_hex(16))

    def rotate(self, identity: ChatIdentity) -> ChatIdentity:
        return ChatIdentity(session_id=secrets.token_hex(16), browser_id=identity.browser_id)

    def issue(self, identity: ChatIdentity) -> str:
        payload = json.dumps(
            {
                "v": 1,
                "s": identity.session_id,
                "b": identity.browser_id,
                "e": int((self._now() + COOKIE_LIFETIME).timestamp()),
            },
            separators=(",", ":"),
        ).encode("utf-8")
        digest = hmac.digest(self._secret, payload, "sha256")
        return f"{_encode(payload)}.{_encode(digest)}"

    def verify(self, token: str | None) -> ChatIdentity | None:
        if token is None or len(token) > 1024 or token.count(".") != 1:
            return None
        encoded_payload, encoded_digest = token.split(".", maxsplit=1)
        try:
            payload = _decode(encoded_payload)
            digest = _decode(encoded_digest)
            value: object = json.loads(payload)
        except (ValueError, UnicodeDecodeError):
            return None
        if not hmac.compare_digest(hmac.digest(self._secret, payload, "sha256"), digest):
            return None
        if not isinstance(value, dict) or set(value) != {"v", "s", "b", "e"}:
            return None
        if value["v"] != 1 or type(value["e"]) is not int:
            return None
        expiry = cast(int, value["e"])
        now = self._now().timestamp()
        if not now < expiry <= now + COOKIE_LIFETIME.total_seconds():
            return None
        session_id, browser_id = value["s"], value["b"]
        if not _generated_id(session_id) or not _generated_id(browser_id):
            return None
        return ChatIdentity(session_id=session_id, browser_id=browser_id)

    def evaluation_token(self) -> str:
        """Derive a separate bearer token for Assay's offline target calls."""
        return hmac.digest(self._secret, b"assay-demo-evaluation-v1", "sha256").hex()

    def valid_evaluation_token(self, value: str | None) -> bool:
        return value is not None and hmac.compare_digest(value, self.evaluation_token())


def _generated_id(value: object) -> bool:
    return (
        isinstance(value, str) and len(value) == 32 and all(c in "0123456789abcdef" for c in value)
    )


def _encode(value: bytes) -> str:
    return base64.urlsafe_b64encode(value).rstrip(b"=").decode("ascii")


def _decode(value: str) -> bytes:
    return base64.b64decode(value + "=" * (-len(value) % 4), altchars=b"-_", validate=True)


def captured_turn(attributes: Mapping[str, object]) -> tuple[str | None, str | None]:
    """Read only the root's first user/assistant text; never child model history."""
    return (
        _message_text(attributes.get("gen_ai.input.messages"), "user"),
        _message_text(attributes.get("gen_ai.output.messages"), "assistant"),
    )


def _message_text(value: object, role: str) -> str | None:
    if isinstance(value, str):
        try:
            value = json.loads(value)
        except ValueError:
            return None
    if not isinstance(value, list):
        return None
    for message in value:
        if not isinstance(message, dict) or message.get("role") != role:
            continue
        content = message.get("content")
        if isinstance(content, str) and content.strip():
            return content
        text = _parts_text(message.get("parts"))
        if text is not None:
            return text
    return None


def _parts_text(value: object) -> str | None:
    if not isinstance(value, list):
        return None
    texts: list[str] = []
    for part in value:
        if not isinstance(part, dict) or part.get("type") != "text":
            continue
        content = part.get("content")
        if isinstance(content, str):
            texts.append(content)
    return "\n".join(texts) if any(text.strip() for text in texts) else None
