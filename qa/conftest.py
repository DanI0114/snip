import os

import pytest
from playwright.sync_api import APIRequestContext, Playwright

@pytest.fixture(scope="session")
def base_url():
    return os.getenv(
        "BASE_URL",
        "https://snip-mm8n.onrender.com",
    ).rstrip("/")


@pytest.fixture
def api_context(
    playwright: Playwright,
    base_url: str,
):
    context = playwright.request.new_context(
        base_url=base_url
    )

    yield context

    context.dispose()

@pytest.fixture(scope="session")
def test_user_email():
    email = os.getenv("TEST_USER_EMAIL")

    if not email:
        pytest.fail("TEST_USER_EMAIL environment variable is required")

    return email


@pytest.fixture(scope="session")
def test_user_password():
    password = os.getenv("TEST_USER_PASSWORD")

    if not password:
        pytest.fail("TEST_USER_PASSWORD environment variable is required")

    return password


@pytest.fixture
def authenticated_api_context(
    playwright: Playwright,
    base_url: str,
    test_user_email: str,
    test_user_password: str,
):
    context = playwright.request.new_context(
        base_url=base_url
    )

    response = context.post(
        "/api/auth/login",
        data={
            "email": test_user_email,
            "password": test_user_password,
        },
    )

    if response.status != 200:
        context.dispose()

        pytest.fail(
            f"Login failed: "
            f"{response.status} {response.text()}"
        )

    yield context

    context.dispose()

@pytest.fixture
def authenticated_api_context_b(
    playwright: Playwright,
    base_url: str,
):
    email = os.getenv("TEST_USER_B_EMAIL")
    password = os.getenv("TEST_USER_B_PASSWORD")

    if not email or not password:
        pytest.skip(
            "Second test user credentials are not configured"
        )

    context = playwright.request.new_context(
        base_url=base_url
    )

    response = context.post(
        "/api/auth/login",
        data={
            "email": email,
            "password": password,
        },
    )

    assert response.status == 200, (
        f"Second user login failed: "
        f"{response.status} {response.text()}"
    )

    yield context

    context.dispose()