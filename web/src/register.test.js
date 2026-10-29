import { describe, it, expect, beforeAll, vi } from "vitest";

// app.js registers the worker once, at load. Each case loads its own copy
// (the query string makes it a separate module) against a different page.
const register = vi.fn(() => Promise.resolve());

beforeAll(async () => {
  Object.defineProperty(navigator, "serviceWorker", { value: { register }, configurable: true });
  await import("./dateformat.js");
});

async function loadApp(body, lang, load) {
  register.mockClear();
  document.documentElement.lang = lang;
  document.body.innerHTML = body;
  await load();
}

describe("service worker registration", () => {
  it("registers with the page locale once signed in", async () => {
    await loadApp(`<details class="usermenu"></details>`, "el", () => import("./app.js?signed-in"));
    expect(register).toHaveBeenCalledWith("/sw.js?lang=el");
  });

  it("does not register from the login page", async () => {
    await loadApp(`<form action="/login"></form>`, "en", () => import("./app.js?signed-out"));
    expect(register).not.toHaveBeenCalled();
  });
});
