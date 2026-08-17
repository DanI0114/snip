async function readJSON(response) {
  try {
    return await response.json();
  } catch {
    return {};
  }
}

function setButtonLoading(button, isLoading, loadingText, normalText) {
  if (!button) return;

  button.disabled = isLoading;
  button.textContent = isLoading ? loadingText : normalText;
}

function escapeHTML(value) {
  return String(value ?? "")
    .replaceAll("&", "&amp;")
    .replaceAll("<", "&lt;")
    .replaceAll(">", "&gt;")
    .replaceAll('"', "&quot;")
    .replaceAll("'", "&#039;");
}

const shortenForm = document.querySelector("#shorten-form");

if (shortenForm) {
  const urlInput = document.querySelector("#url");
  const errorElement = document.querySelector("#error");
  const resultElement = document.querySelector("#result");
  const shortURLLink = document.querySelector("#short-url");
  const copyButton = document.querySelector("#copy-button");
  const submitButton = shortenForm.querySelector('button[type="submit"]');

  shortenForm.addEventListener("submit", async (event) => {
    event.preventDefault();

    errorElement.textContent = "";
    resultElement.hidden = true;

    setButtonLoading(submitButton, true, "Shrinking...", "Shrink it");

    try {
      const response = await fetch("/api/links", {
        method: "POST",
        headers: {
          "Content-Type": "application/json",
        },
        body: JSON.stringify({
          url: urlInput.value.trim(),
        }),
      });

      const data = await readJSON(response);

      if (response.status === 401) {
        throw new Error("Please sign in to shorten links");
      }

      if (!response.ok) {
        throw new Error(data.error || "Could not shorten the URL");
      }

      shortURLLink.href = data.short_url;
      shortURLLink.textContent = data.short_url;
      resultElement.hidden = false;
    } catch (error) {
      errorElement.textContent = error.message;
    } finally {
      setButtonLoading(submitButton, false, "Shrinking...", "Shrink it");
    }
  });

  if (copyButton) {
    copyButton.addEventListener("click", async () => {
      try {
        await navigator.clipboard.writeText(shortURLLink.href);

        copyButton.textContent = "Copied";

        setTimeout(() => {
          copyButton.textContent = "Copy";
        }, 1500);
      } catch {
        errorElement.textContent = "Could not copy the link";
      }
    });
  }
}

const loginForm = document.querySelector("#login-form");

if (loginForm) {
  const errorElement = document.querySelector("#error");
  const successElement = document.querySelector("#success");
  const submitButton = loginForm.querySelector('button[type="submit"]');

  const query = new URLSearchParams(window.location.search);

  if (query.get("verified") === "1" && successElement) {
    successElement.textContent =
      "Email verified successfully. You can now sign in.";
  }

  if (query.get("verification") === "invalid") {
    errorElement.textContent =
      "That verification link is invalid or has expired.";
  }

  loginForm.addEventListener("submit", async (event) => {
    event.preventDefault();

    errorElement.textContent = "";

    if (successElement) {
      successElement.textContent = "";
    }

    const email = document.querySelector("#email").value.trim();
    const password = document.querySelector("#password").value;

    setButtonLoading(
      submitButton,
      true,
      "Signing in...",
      "Sign in"
    );

    try {
      const response = await fetch("/api/auth/login", {
        method: "POST",
        headers: {
          "Content-Type": "application/json",
        },
        body: JSON.stringify({
          email,
          password,
        }),
      });

      const data = await readJSON(response);

      if (!response.ok) {
        throw new Error(data.error || "Could not sign in");
      }

      window.location.href = "/";
    } catch (error) {
      errorElement.textContent = error.message;

      setButtonLoading(
        submitButton,
        false,
        "Signing in...",
        "Sign in"
      );
    }
  });
}

const registerForm = document.querySelector("#register-form");

if (registerForm) {
  const successElement = document.querySelector("#success");
  const errorElement = document.querySelector("#error");
  const submitButton = registerForm.querySelector('button[type="submit"]');

  registerForm.addEventListener("submit", async (event) => {
    event.preventDefault();

    errorElement.textContent = "";

    const name = document.querySelector("#name").value.trim();
    const email = document.querySelector("#email").value.trim();
    const password = document.querySelector("#password").value;
    const confirmPassword =
      document.querySelector("#confirm-password").value;

    if (password !== confirmPassword) {
      errorElement.textContent = "Passwords don't match";
      return;
    }

    setButtonLoading(
      submitButton,
      true,
      "Creating account...",
      "Create account"
    );

    try {
      const response = await fetch("/api/auth/register", {
        method: "POST",
        headers: {
          "Content-Type": "application/json",
        },
        body: JSON.stringify({
          name,
          email,
          password,
          password_confirmation: confirmPassword,
        }),
      });

      const data = await readJSON(response);

      if (!response.ok) {
        throw new Error(data.error || "Could not create account");
      }

      successElement.textContent =
         data.message || "Check your email for the verification link.";

        registerForm.reset();

        submitButton.disabled = true;
        submitButton.textContent = "Check your email";
    } catch (error) {
      errorElement.textContent = error.message;

      setButtonLoading(
        submitButton,
        false,
        "Creating account...",
        "Create account"
      );
    }
  });
}

function getAuthActionsContainer() {
  const explicitContainer = document.querySelector("#auth-actions");

  if (explicitContainer) {
    return explicitContainer;
  }

  if (shortenForm) {
    return document.querySelector(".header-actions");
  }

  return null;
}

async function loadAuthState() {
  const authActions = getAuthActionsContainer();

  if (!authActions) {
    return;
  }

  // Show usable navigation immediately.
  renderLoggedOut(authActions);

  try {
    const response = await fetch("/api/auth/me");

    if (response.status === 401) {
      return;
    }

    const user = await readJSON(response);

    if (!response.ok) {
      console.error("Could not load authentication state:", user);
      return;
    }

    renderLoggedIn(authActions, user);
  } catch (error) {
    console.error("Authentication state error:", error);
  }
}

function renderLoggedOut(container) {
  container.innerHTML = `
    <a class="btn btn-ghost" href="/login">Sign in</a>
    <a class="btn btn-primary" href="/register">Sign up</a>
  `;
}

function renderLoggedIn(container, user) {
  const displayName =
    user.name ||
    (user.user_id ? `User #${user.user_id}` : "Account");

  const email = user.email || "";
  const initial = displayName.trim().charAt(0).toUpperCase() || "?";

  container.innerHTML = `
    <div class="user-menu" id="user-menu">
      <button
        class="user-menu-trigger"
        id="user-menu-trigger"
        type="button"
        aria-haspopup="true"
        aria-expanded="false"
      >
        <span class="user-avatar">
          ${escapeHTML(initial)}
        </span>

        <span class="user-name">
          ${escapeHTML(displayName)}
        </span>

        <span class="user-chevron" aria-hidden="true">
          ▼
        </span>
      </button>

      <div class="user-dropdown">
        <div class="user-dropdown-header">
          <div class="user-dropdown-name">
            ${escapeHTML(displayName)}
          </div>

          ${
            email
              ? `
                <div class="user-dropdown-email">
                  ${escapeHTML(email)}
                </div>
              `
              : ""
          }
        </div>

        <div class="user-dropdown-divider"></div>

        <a
          class="user-dropdown-item"
          href="/my-links"
        >
          My links
        </a>

        <div class="user-dropdown-divider"></div>

        <button
          class="user-dropdown-item logout"
          id="logout-button"
          type="button"
        >
          Log out
        </button>
      </div>
    </div>
  `;

  setupUserMenu();
  setupLogout();
}

function setupUserMenu() {
  const menu = document.querySelector("#user-menu");
  const trigger = document.querySelector("#user-menu-trigger");

  if (!menu || !trigger) {
    return;
  }

  function closeMenu() {
    menu.classList.remove("open");
    trigger.setAttribute("aria-expanded", "false");
  }

  trigger.addEventListener("click", (event) => {
    event.stopPropagation();

    const open = menu.classList.toggle("open");

    trigger.setAttribute(
      "aria-expanded",
      String(open)
    );
  });

  document.addEventListener("click", (event) => {
    if (!menu.contains(event.target)) {
      closeMenu();
    }
  });

  document.addEventListener("keydown", (event) => {
    if (event.key === "Escape") {
      closeMenu();
    }
  });
}

function setupLogout() {
  const logoutButton = document.querySelector("#logout-button");

  if (!logoutButton) {
    return;
  }

  logoutButton.addEventListener("click", async () => {
    setButtonLoading(
      logoutButton,
      true,
      "Logging out...",
      "Log out"
    );

    try {
      const response = await fetch("/api/auth/logout", {
        method: "POST",
      });

      const data = await readJSON(response);

      if (!response.ok) {
        throw new Error(data.error || "Could not log out");
      }

      window.location.href = "/";
    } catch (error) {
      console.error("Logout failed:", error);

      setButtonLoading(
        logoutButton,
        false,
        "Logging out...",
        "Log out"
      );
    }
  });
}

const linksList = document.querySelector("#links-list");

if (linksList) {
  loadMyLinks();
}

async function loadMyLinks() {
  const loadingElement =
    document.querySelector("#links-loading");

  const emptyElement =
    document.querySelector("#links-empty");

  const errorElement =
    document.querySelector("#links-error");

  const summaryElement =
    document.querySelector("#links-summary");

  const totalLinksElement =
    document.querySelector("#total-links");

  const totalClicksElement =
    document.querySelector("#total-clicks");

  try {
    const response = await fetch("/api/links/mine");

    if (response.status === 401) {
      window.location.href = "/login";
      return;
    }

    const data = await readJSON(response);

    if (!response.ok) {
      throw new Error(
        data.error || "Could not load your links"
      );
    }

    const links = data.links || [];

    loadingElement.hidden = true;

    if (links.length === 0) {
      emptyElement.hidden = false;
      return;
    }

    const totalClicks = links.reduce(
      (sum, link) => sum + link.clicks,
      0
    );

    totalLinksElement.textContent =
      links.length.toLocaleString();

    totalClicksElement.textContent =
      totalClicks.toLocaleString();

    summaryElement.hidden = false;

    renderMyLinks(links);

  } catch (error) {
    loadingElement.hidden = true;
    errorElement.textContent = error.message;
  }
}

function renderMyLinks(links) {
  linksList.innerHTML = links
    .map((link) => {
      const createdAt = new Date(
        link.created_at
      ).toLocaleDateString(undefined, {
        year: "numeric",
        month: "short",
        day: "numeric",
      });

      return `
        <article
          class="link-card"
          data-link-code="${escapeHTML(link.code)}"
          data-clicks="${Number(link.clicks)}"
        >

          <div class="link-card-main">
            <a
              class="link-short-url"
              href="${escapeHTML(link.short_url)}"
              target="_blank"
              rel="noopener"
            >
              ${escapeHTML(link.short_url)}
            </a>

            <a
              class="link-target-url"
              href="${escapeHTML(link.target_url)}"
              target="_blank"
              rel="noopener"
              title="${escapeHTML(link.target_url)}"
            >
              ${escapeHTML(link.target_url)}
            </a>

          </div>


          <div class="link-card-meta">

            <div class="link-clicks">
              <strong>
                ${Number(link.clicks).toLocaleString()}
              </strong>

              <span>
                ${Number(link.clicks) === 1 ? "click" : "clicks"}
              </span>
            </div>


            <div class="link-date">
              ${escapeHTML(createdAt)}
            </div>


            <div class="link-actions">

              <button
                class="btn btn-ghost link-copy-button"
                type="button"
                data-copy-url="${escapeHTML(link.short_url)}"
              >
                Copy
              </button>

              <button
                class="btn btn-danger link-delete-button"
                type="button"
                data-code="${escapeHTML(link.code)}"
              >
                Delete
              </button>

            </div>

          </div>

        </article>
      `;
    })
    .join("");

  linksList.hidden = false;

  setupLinkCopyButtons();
  setupLinkDeleteButtons();
}

function setupLinkCopyButtons() {
  const buttons =
    document.querySelectorAll(".link-copy-button");

  buttons.forEach((button) => {

    button.addEventListener("click", async () => {

      const url = button.dataset.copyUrl;

      try {
        await navigator.clipboard.writeText(url);

        button.textContent = "Copied";

        setTimeout(() => {
          button.textContent = "Copy";
        }, 1500);

      } catch {
        button.textContent = "Failed";

        setTimeout(() => {
          button.textContent = "Copy";
        }, 1500);
      }

    });

  });
}

function setupLinkDeleteButtons() {
  const buttons =
    document.querySelectorAll(".link-delete-button");

  buttons.forEach((button) => {
    button.addEventListener("click", async () => {
      const code = button.dataset.code;

      const confirmed = window.confirm(
        "Delete this short link? This cannot be undone."
      );

      if (!confirmed) {
        return;
      }

      const originalText = button.textContent;

      button.disabled = true;
      button.textContent = "Deleting...";

      try {
        const response = await fetch(
          `/api/links/${encodeURIComponent(code)}`,
          {
            method: "DELETE",
          }
        );

        const data = await readJSON(response);

        if (response.status === 401) {
          window.location.href = "/login";
          return;
        }

        if (!response.ok) {
          throw new Error(
            data.error || "Could not delete link"
          );
        }

        const card = document.querySelector(
          `[data-link-code="${CSS.escape(code)}"]`
        );

        if (card) {
          card.remove();
        }

        updateDashboardAfterDelete();

      } catch (error) {
        console.error("Delete link failed:", error);

        button.disabled = false;
        button.textContent = originalText;

        alert(error.message);
      }
    });
  });
}

function updateDashboardAfterDelete() {
  const cards =
    document.querySelectorAll(".link-card");

  const totalLinksElement =
    document.querySelector("#total-links");

  const totalClicksElement =
    document.querySelector("#total-clicks");

  const linksListElement =
    document.querySelector("#links-list");

  const emptyElement =
    document.querySelector("#links-empty");

  const summaryElement =
    document.querySelector("#links-summary");


  // ------------------------------------------
  // Recalculate total clicks
  // ------------------------------------------

  const totalClicks = Array
    .from(cards)
    .reduce((sum, card) => {
      return sum + Number(
        card.dataset.clicks || 0
      );
    }, 0);


  // ------------------------------------------
  // Update counters
  // ------------------------------------------

  if (totalLinksElement) {
    totalLinksElement.textContent =
      cards.length.toLocaleString();
  }

  if (totalClicksElement) {
    totalClicksElement.textContent =
      totalClicks.toLocaleString();
  }


  // ------------------------------------------
  // If there are no links left,
  // show the empty state
  // ------------------------------------------

  if (cards.length === 0) {
    if (linksListElement) {
      linksListElement.hidden = true;
    }

    if (summaryElement) {
      summaryElement.hidden = true;
    }

    if (emptyElement) {
      emptyElement.hidden = false;
    }

    return;
  }


  // ------------------------------------------
  // Otherwise keep dashboard visible
  // ------------------------------------------

  if (linksListElement) {
    linksListElement.hidden = false;
  }

  if (summaryElement) {
    summaryElement.hidden = false;
  }

  if (emptyElement) {
    emptyElement.hidden = true;
  }
}

loadAuthState();