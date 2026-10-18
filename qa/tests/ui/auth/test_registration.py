from playwright.sync_api import Page, expect


def test_registration_rejects_mismatching_passwords(
    page: Page,
    base_url: str
):
    page.goto(f"{base_url}/register")

    page.get_by_label("Name").fill("QA Test User")
    page.get_by_label("Email").fill("qa-test@example.com")
    page.get_by_label("Password", exact=True).fill(
        "VeryStrongPassword123!"
    )
    page.get_by_label("Confirm Password").fill(
        "DifferentPassword123!"
    )

    page.get_by_role(
        "button",
        name="Create account"
    ).click()

    expect(
        page.get_by_text("Passwords don't match")
    ).to_be_visible()