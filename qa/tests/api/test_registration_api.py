import uuid

import pytest
from playwright.sync_api import APIRequestContext


def unique_email() -> str:
    return f"qa-{uuid.uuid4().hex}@example.com"


def registration_payload(
    *,
    name="QA Test User",
    email=None,
    password="A" * 15,
    password_confirmation=None,
):
    if email is None:
        email = unique_email()

    if password_confirmation is None:
        password_confirmation = password

    return {
        "name": name,
        "email": email,
        "password": password,
        "password_confirmation": password_confirmation,
    }


@pytest.mark.parametrize(
    "password, expected_error",
    [
        ("A" * 14, "password must be at least 15 characters"),
        ("A" * 129, "password is too long"),
    ],
    ids=[
        "below_minimum",
        "above_maximum",
    ],
)
def test_registration_rejects_invalid_password_length(
    api_context: APIRequestContext,
    password: str,
    expected_error: str,
):
    response = api_context.post(
        "/api/auth/register",
        data=registration_payload(password=password),
    )

    assert response.status == 422
    assert response.json()["error"] == expected_error


def test_registration_rejects_password_mismatch(
    api_context: APIRequestContext,
):
    response = api_context.post(
        "/api/auth/register",
        data=registration_payload(
            password="A" * 15,
            password_confirmation="B" * 15,
        ),
    )

    assert response.status == 422
    assert response.json() == {
        "error": "passwords do not match"
    }


@pytest.mark.parametrize(
    "email",
    [
        "",
        "not-an-email",
        "hello@@example.com",
    ],
)
def test_registration_rejects_invalid_email(
    api_context: APIRequestContext,
    email: str,
):
    response = api_context.post(
        "/api/auth/register",
        data=registration_payload(email=email),
    )

    assert response.status == 422
    assert response.json() == {
        "error": "invalid email address"
    }


def test_registration_rejects_empty_name(
    api_context: APIRequestContext,
):
    response = api_context.post(
        "/api/auth/register",
        data=registration_payload(name=""),
    )

    assert response.status == 422
    assert response.json() == {
        "error": "name is required"
    }


def test_registration_rejects_whitespace_only_name(
    api_context: APIRequestContext,
):
    response = api_context.post(
        "/api/auth/register",
        data=registration_payload(name="     "),
    )

    assert response.status == 422
    assert response.json() == {
        "error": "name is required"
    }


def test_registration_rejects_name_over_64_characters(
    api_context: APIRequestContext,
):
    response = api_context.post(
        "/api/auth/register",
        data=registration_payload(name="A" * 65),
    )

    assert response.status == 422
    assert response.json() == {
        "error": "name is too long"
    }


def test_registration_rejects_unknown_json_field(
    api_context: APIRequestContext,
):
    payload = registration_payload()
    payload["admin"] = True

    response = api_context.post(
        "/api/auth/register",
        data=payload,
    )

    assert response.status == 400
    assert response.json() == {
        "error": "invalid request body"
    }

def test_registration_rejects_malformed_json(
    api_context: APIRequestContext,
):
    response = api_context.post(
        "/api/auth/register",
        data='{"name": "broken"',
        headers={
            "Content-Type": "application/json"
        },
    )

    assert response.status == 400
    assert response.json() == {
        "error": "invalid request body"
    }

def test_registration_rejects_missing_name(
    api_context: APIRequestContext,
):
    payload = {
        "email": unique_email(),
        "password": "A" * 15,
        "password_confirmation": "A" * 15,
    }

    response = api_context.post(
        "/api/auth/register",
        data=payload,
    )

    assert response.status == 422
    assert response.json()["error"] == "name is required"
