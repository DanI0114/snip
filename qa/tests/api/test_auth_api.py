import uuid

from playwright.sync_api import APIRequestContext


def test_login_with_valid_credentials(
    api_context: APIRequestContext,
    test_user_email: str,
    test_user_password: str,
):
    response = api_context.post(
        "/api/auth/login",
        data={
            "email": test_user_email,
            "password": test_user_password,
        },
    )

    assert response.status == 200


def test_login_rejects_wrong_password(
    api_context: APIRequestContext,
    test_user_email: str,
):
    response = api_context.post(
        "/api/auth/login",
        data={
            "email": test_user_email,
            "password": "DefinitelyWrongPassword123!",
        },
    )

    print(response.status)
    print(response.text())

    assert response.status == 401


def test_login_rejects_nonexistent_user(
    api_context: APIRequestContext,
):
    email = f"does-not-exist-{uuid.uuid4().hex}@example.com"

    response = api_context.post(
        "/api/auth/login",
        data={
            "email": email,
            "password": "DefinitelyWrongPassword123!",
        },
    )

    print(response.status)
    print(response.text())

    assert response.status == 401

def test_authenticated_user_can_get_current_user(
    authenticated_api_context: APIRequestContext,
):
    response = authenticated_api_context.get(
        "/api/auth/me"
    )

    print(response.status)
    print(response.text())

    assert response.status == 200

    body = response.json()

    assert "email" in body

def test_me_rejects_unauthenticated_request(
    api_context: APIRequestContext,
):
    response = api_context.get(
        "/api/auth/me"
    )

    assert response.status == 401

def test_logout_invalidates_session(
    authenticated_api_context: APIRequestContext,
):
    before_logout = authenticated_api_context.get(
        "/api/auth/me"
    )

    assert before_logout.status == 200

    logout_response = authenticated_api_context.post(
        "/api/auth/logout"
    )

    assert logout_response.status == 200
    assert logout_response.json()["message"] == "logged out"

    after_logout = authenticated_api_context.get(
        "/api/auth/me"
    )

    assert after_logout.status == 401

def test_login_does_not_reveal_whether_account_exists(
    api_context: APIRequestContext,
    test_user_email: str,
):
    wrong_password_response = api_context.post(
        "/api/auth/login",
        data={
            "email": test_user_email,
            "password": "DefinitelyWrongPassword123!",
        },
    )

    nonexistent_response = api_context.post(
        "/api/auth/login",
        data={
            "email": f"{uuid.uuid4().hex}@example.com",
            "password": "DefinitelyWrongPassword123!",
        },
    )

    assert wrong_password_response.status == nonexistent_response.status

    assert (
        wrong_password_response.json()["error"]
        ==
        nonexistent_response.json()["error"]
    )

def test_login_rejects_malformed_json(
    api_context: APIRequestContext,
):
    response = api_context.post(
        "/api/auth/login",
        data='{"email": "broken"',
        headers={
            "Content-Type": "application/json",
        },
    )

    assert response.status == 400

def test_login_rejects_missing_password(
    api_context: APIRequestContext,
):
    response = api_context.post(
        "/api/auth/login",
        data={
            "email": "someone@example.com",
        },
    )

    assert response.status == 401
    assert response.json() == {
        "error": "invalid email or password"
    }