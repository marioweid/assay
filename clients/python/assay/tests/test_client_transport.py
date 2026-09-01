from typing import Literal

import httpx
import pytest

from assay.client import Client, _Transport
from assay.exceptions import (
    AssayAPIError,
    AssayConfigurationError,
    AssayProtocolError,
    AssayTimeoutError,
    AssayTransportError,
)


def test_ready_accepts_plain_text_without_credentials() -> None:
    requests: list[httpx.Request] = []

    def handler(request: httpx.Request) -> httpx.Response:
        requests.append(request)
        return httpx.Response(200, text="ready\n")

    http_client = httpx.Client(transport=httpx.MockTransport(handler))
    with Client("https://assay.test/", _http_client=http_client) as client:
        client.ready()

    assert requests[0].url == httpx.URL("https://assay.test/readyz")
    assert "authorization" not in requests[0].headers
    assert "x-api-key" not in requests[0].headers
    assert requests[0].headers["user-agent"] == "assay-sdk/0.2.0"
    assert not http_client.is_closed
    http_client.close()


@pytest.mark.parametrize(
    ("auth", "expected_header", "expected_value"),
    [
        ("admin", "authorization", "Bearer admin-secret"),
        ("project", "x-api-key", "asy_secret"),
    ],
)
def test_transport_selects_exactly_one_credential(
    auth: Literal["admin", "project"],
    expected_header: str,
    expected_value: str,
) -> None:
    requests: list[httpx.Request] = []

    def handler(request: httpx.Request) -> httpx.Response:
        requests.append(request)
        return httpx.Response(200, json={"items": []})

    http_client = httpx.Client(transport=httpx.MockTransport(handler))
    transport = _Transport(
        "https://assay.test",
        api_key="asy_secret",
        admin_token="admin-secret",
        timeout=10.0,
        http_client=http_client,
    )

    result = transport.request("list", "GET", "/v1/items", auth=auth)

    assert result == {"items": []}
    assert requests[0].headers[expected_header] == expected_value
    assert requests[0].headers["user-agent"] == "assay-sdk/0.2.0"
    other_header = "x-api-key" if expected_header == "authorization" else "authorization"
    assert other_header not in requests[0].headers
    http_client.close()


@pytest.mark.parametrize("auth", ["admin", "project"])
def test_transport_rejects_missing_credential_before_io(
    auth: Literal["admin", "project"],
) -> None:
    called = False

    def handler(_: httpx.Request) -> httpx.Response:
        nonlocal called
        called = True
        return httpx.Response(200, json={})

    http_client = httpx.Client(transport=httpx.MockTransport(handler))
    transport = _Transport(
        "https://assay.test",
        api_key=None,
        admin_token=None,
        timeout=10.0,
        http_client=http_client,
    )

    with pytest.raises(AssayConfigurationError, match=f"{auth} credential is required"):
        transport.request("operation", "GET", "/v1/items", auth=auth)

    assert not called
    http_client.close()


def test_transport_accepts_no_content() -> None:
    http_client = httpx.Client(transport=httpx.MockTransport(lambda _: httpx.Response(204)))
    transport = _Transport(
        "https://assay.test",
        api_key=None,
        admin_token="admin-secret",
        timeout=10.0,
        http_client=http_client,
    )

    assert transport.request("delete", "DELETE", "/v1/item", auth="admin") is None
    http_client.close()


@pytest.mark.parametrize("body", [b"not-json", b"[]", b"null"])
def test_transport_rejects_malformed_success_payload(body: bytes) -> None:
    http_client = httpx.Client(
        transport=httpx.MockTransport(
            lambda _: httpx.Response(
                200, content=body, headers={"content-type": "application/json"}
            )
        )
    )
    transport = _Transport(
        "https://assay.test",
        api_key=None,
        admin_token="admin-secret",
        timeout=10.0,
        http_client=http_client,
    )

    with pytest.raises(AssayProtocolError, match="operation: invalid JSON response"):
        transport.request("operation", "GET", "/v1/item", auth="admin")
    http_client.close()


def test_transport_maps_problem_details_without_retaining_response() -> None:
    http_client = httpx.Client(
        transport=httpx.MockTransport(
            lambda _: httpx.Response(
                400,
                json={"title": "Bad Request", "detail": "invalid name", "secret": "do-not-copy"},
            )
        )
    )
    transport = _Transport(
        "https://assay.test",
        api_key=None,
        admin_token="admin-secret",
        timeout=10.0,
        http_client=http_client,
    )

    with pytest.raises(AssayAPIError) as captured:
        transport.request("create project", "POST", "/v1/projects", auth="admin")

    error = captured.value
    assert error.status_code == 400
    assert str(error) == "create project: HTTP 400 Bad Request: invalid name"
    assert "do-not-copy" not in repr(error.__dict__)
    http_client.close()


def test_transport_omits_non_problem_error_body() -> None:
    http_client = httpx.Client(
        transport=httpx.MockTransport(lambda _: httpx.Response(500, text="sensitive response"))
    )
    transport = _Transport(
        "https://assay.test",
        api_key=None,
        admin_token="admin-secret",
        timeout=10.0,
        http_client=http_client,
    )

    with pytest.raises(AssayAPIError, match="operation: HTTP 500") as captured:
        transport.request("operation", "GET", "/v1/item", auth="admin")
    assert "sensitive" not in str(captured.value)
    http_client.close()


@pytest.mark.parametrize(
    ("failure", "error_type"),
    [
        (httpx.ReadTimeout("secret"), AssayTimeoutError),
        (httpx.ConnectError("secret"), AssayTransportError),
    ],
)
def test_transport_maps_request_failures_without_underlying_message(
    failure: httpx.RequestError,
    error_type: type[Exception],
) -> None:
    def handler(request: httpx.Request) -> httpx.Response:
        failure.request = request
        raise failure

    http_client = httpx.Client(transport=httpx.MockTransport(handler))
    transport = _Transport(
        "https://assay.test",
        api_key=None,
        admin_token="admin-secret",
        timeout=10.0,
        http_client=http_client,
    )

    with pytest.raises(error_type, match="operation: request failed") as captured:
        transport.request("operation", "GET", "/v1/item", auth="admin")
    assert "secret" not in str(captured.value)
    assert captured.value.__context__ is None
    http_client.close()


@pytest.mark.parametrize(
    "endpoint",
    ["", "assay.test", "ftp://assay.test", "https://assay.test/path", "https://assay.test?q=1"],
)
def test_client_rejects_invalid_base_endpoint(endpoint: str) -> None:
    with pytest.raises(AssayConfigurationError, match="endpoint must be an HTTP or HTTPS base URL"):
        Client(endpoint)


@pytest.mark.parametrize("timeout", [0.0, -1.0, float("inf"), float("nan")])
def test_client_rejects_invalid_timeout(timeout: float) -> None:
    with pytest.raises(AssayConfigurationError, match="timeout must be a positive finite number"):
        Client("https://assay.test", timeout=timeout)
