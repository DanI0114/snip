import pytest
from playwright.sync_api import APIRequestContext


def create_link(
    api_context: APIRequestContext,
    url: str = "https://example.com",
):
    response = api_context.post(
        "/api/links",
        data={
            "url": url,
        },
    )

    assert response.status == 201

    body = response.json()

    assert "code" in body
    assert "short_url" in body
    assert len(body["code"]) == 7

    return body


def test_authenticated_user_can_create_link(
    authenticated_api_context: APIRequestContext,
):
    body = create_link(
        authenticated_api_context,
        "https://example.com",
    )

    assert body["short_url"].endswith(
        f"/{body['code']}"
    )

def test_unauthenticated_user_cannot_create_link(
    api_context: APIRequestContext,
):
    response = api_context.post(
        "/api/links",
        data={
            "url": "https://example.com",
        },
    )

    assert response.status == 401

@pytest.mark.parametrize(
    "url, expected_error",
    [
        (
            "",
            "URL is required",
        ),
        (
            "ftp://example.com",
            "only http and https URLs are allowed",
        ),
        (
            "https://",
            "URL must include a hostname",
        ),
    ],
    ids=[
        "empty_url",
        "unsupported_scheme",
        "missing_hostname",
    ],
)
def test_create_link_rejects_invalid_url(
    authenticated_api_context: APIRequestContext,
    url: str,
    expected_error: str,
):
    response = authenticated_api_context.post(
        "/api/links",
        data={
            "url": url,
        },
    )

    assert response.status == 422
    assert response.json() == {
        "error": expected_error
    }

def test_create_link_rejects_url_over_maximum_length(
    authenticated_api_context: APIRequestContext,
):
    url = "https://example.com/" + ("a" * 2048)

    response = authenticated_api_context.post(
        "/api/links",
        data={
            "url": url,
        },
    )

    assert response.status == 422
    assert response.json() == {
        "error": "URL is too long"
    }

def test_short_link_redirects_to_original_url(
    authenticated_api_context: APIRequestContext,
    api_context: APIRequestContext,
):
    body = create_link(
        authenticated_api_context,
        "https://example.com",
    )

    code = body["code"]

    response = api_context.get(
        f"/{code}",
        max_redirects=0,
    )

    assert response.status == 302
    assert response.headers["location"] == "https://example.com"

def test_url_without_scheme_is_normalized_to_https(
    authenticated_api_context: APIRequestContext,
    api_context: APIRequestContext,
):
    body = create_link(
        authenticated_api_context,
        "example.com",
    )

    response = api_context.get(
        f"/{body['code']}",
        max_redirects=0,
    )

    assert response.status == 302
    assert response.headers["location"] == "https://example.com"

def test_user_can_delete_own_link(
    authenticated_api_context: APIRequestContext,
    api_context: APIRequestContext,
):
    body = create_link(
        authenticated_api_context,
    )

    code = body["code"]

    delete_response = authenticated_api_context.delete(
        f"/api/links/{code}"
    )

    assert delete_response.status == 200
    assert delete_response.json() == {
        "message": "link deleted"
    }

    redirect_response = api_context.get(
        f"/{code}",
        max_redirects=0,
    )

    assert redirect_response.status == 404

def test_delete_rejects_invalid_link_code(
    authenticated_api_context: APIRequestContext,
):
    response = authenticated_api_context.delete(
        "/api/links/abc"
    )

    assert response.status == 400
    assert response.json() == {
        "error": "invalid short link"
    }

def test_user_cannot_delete_another_users_link(
    authenticated_api_context: APIRequestContext,
    authenticated_api_context_b: APIRequestContext,
    api_context: APIRequestContext,
):
    # User A creates the link.
    body = create_link(
        authenticated_api_context,
    )

    code = body["code"]

    # User B attempts to delete User A's link.
    delete_response = authenticated_api_context_b.delete(
        f"/api/links/{code}"
    )

    assert delete_response.status == 404
    assert delete_response.json() == {
        "error": "link not found"
    }

    # Most importantly: User A's link must still work.
    redirect_response = api_context.get(
        f"/{code}",
        max_redirects=0,
    )

    assert redirect_response.status == 302