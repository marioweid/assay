import json
from datetime import UTC, datetime, timedelta

from chat_sessions import ChatIdentity, SessionSigner, captured_turn


def test_signed_capability_round_trip_and_rotation() -> None:
    now = datetime(2026, 9, 26, tzinfo=UTC)
    signer = SessionSigner(b"s" * 32, now=lambda: now)
    identity = signer.create()
    token = signer.issue(identity)
    assert signer.verify(token) == identity
    rotated = signer.rotate(identity)
    assert rotated.browser_id == identity.browser_id
    assert rotated.session_id != identity.session_id
    assert signer.verify(signer.issue(rotated)) == rotated
    assert signer.verify(token) == identity  # Stateless rotation cannot revoke copied tokens.


def test_capability_rejects_forgery_expiry_and_bad_data() -> None:
    now = datetime(2026, 9, 26, tzinfo=UTC)
    signer = SessionSigner(b"s" * 32, now=lambda: now)
    token = signer.issue(signer.create())
    forged = token[:-1] + ("A" if token[-1] != "A" else "B")
    assert signer.verify(forged) is None
    assert signer.verify("bad.token") is None
    assert signer.verify("x" * 2000) is None
    later = SessionSigner(b"s" * 32, now=lambda: now + timedelta(days=31))
    assert later.verify(token) is None
    other = SessionSigner(b"t" * 32, now=lambda: now)
    assert other.verify(token) is None


def test_root_capture_extracts_only_current_pair_and_handles_bad_content() -> None:
    attributes = {
        "gen_ai.input.messages": json.dumps([{"role": "user", "content": "hello"}]),
        "gen_ai.output.messages": json.dumps([{"role": "assistant", "content": "hi"}]),
    }
    assert captured_turn(attributes) == ("hello", "hi")
    assert captured_turn({**attributes, "gen_ai.output.messages": "{bad"}) == ("hello", None)
    assert captured_turn({"gen_ai.input.messages": []}) == (None, None)
    assert captured_turn({"gen_ai.input.messages": "[]"}) == (None, None)


def test_signing_requires_persistent_high_entropy_secret() -> None:
    try:
        SessionSigner(b"short")
    except ValueError as error:
        assert "32 bytes" in str(error)
    else:
        raise AssertionError("short signing secret accepted")
    assert ChatIdentity("session", "browser") == ChatIdentity("session", "browser")
