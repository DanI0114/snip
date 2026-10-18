import re

import pytest
from playwright.sync_api import Page, expect


@pytest.mark.smoke
def test_registration_page_is_available(page: Page, base_url: str):
    page.goto(f"{base_url}/register")

    expect(page.get_by_label("Email")).to_be_visible()
    expect(
        page.get_by_role(
            "button",
            name=re.compile("create account", re.IGNORECASE)
        )
    ).to_be_visible()