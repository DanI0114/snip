const form = document.querySelector("#shorten-form");
const urlInput = document.querySelector("#url");
const errorElement = document.querySelector("#error");
const resultElement = document.querySelector("#result");
const shortURLLink = document.querySelector("#short-url");
const copyButton = document.querySelector("#copy-button");

form.addEventListener("submit", async (event) => {
    event.preventDefault();

    errorElement.textContent = "";
    resultElement.hidden = true;

    try {
        const response = await fetch("/api/links", {
            method: "POST",
            headers: {
                "Content-Type": "application/json",
            },
            body: JSON.stringify({
                url: urlInput.value,
            }),
        });

        const data = await response.json();

        if (!response.ok) {
            throw new Error(data.error || "Could not shorten the URL");
        }

        shortURLLink.href = data.short_url;
        shortURLLink.textContent = data.short_url;
        resultElement.hidden = false;
    } catch (error) {
        errorElement.textContent = error.message;
    }
});

copyButton.addEventListener("click", async () => {
    await navigator.clipboard.writeText(shortURLLink.href);
    copyButton.textContent = "Copied";

    setTimeout(() => {
        copyButton.textContent = "Copy";
    }, 1500);
});