import { describe, it, expect, beforeEach, afterEach, vi } from "vitest";

import "./dateformat.js";
import "./app.js";

function setVisibility(state) {
  Object.defineProperty(document, "visibilityState", { value: state, configurable: true });
  document.dispatchEvent(new window.Event("visibilitychange"));
}

function pageshow(persisted) {
  const ev = new window.Event("pageshow");
  ev.persisted = persisted;
  window.dispatchEvent(ev);
}

beforeEach(() => {
  vi.useFakeTimers();
  window.htmx = { ajax: vi.fn(() => Promise.resolve()) };
});

afterEach(() => {
  vi.useRealTimers();
  delete window.htmx;
  document.body.innerHTML = "";
});

describe("refresh on resume", () => {
  it("leaves the list alone after a quick tab switch", () => {
    document.body.innerHTML = `<div id="task-list"></div>`;
    setVisibility("hidden");
    vi.advanceTimersByTime(5000);
    setVisibility("visible");
    vi.advanceTimersByTime(300);
    expect(window.htmx.ajax).not.toHaveBeenCalled();
  });

  it("reloads the task list after a long absence", () => {
    document.body.innerHTML = `<div id="task-list"></div>`;
    setVisibility("hidden");
    vi.advanceTimersByTime(61000);
    expect(window.htmx.ajax).not.toHaveBeenCalled();
    setVisibility("visible");
    vi.advanceTimersByTime(300);
    expect(window.htmx.ajax).toHaveBeenCalledTimes(1);
    const [method, url, opts] = window.htmx.ajax.mock.calls[0];
    expect(method).toBe("GET");
    expect(url).toBe(window.location.pathname + window.location.search);
    expect(opts).toMatchObject({ target: "#task-list", select: "#task-list", swap: "outerHTML" });
  });

  it("asks for as many rows as are on screen", () => {
    document.body.innerHTML = `<div id="task-list">${'<div class="task-row"></div>'.repeat(40)}</div>`;
    pageshow(true);
    vi.advanceTimersByTime(300);
    const url = new URL(window.htmx.ajax.mock.calls[0][1], "http://x");
    expect(url.searchParams.get("rows")).toBe("40");
  });

  it("reloads the calendar in calendar view", () => {
    document.body.innerHTML = `<div id="calendar"></div>`;
    pageshow(true);
    vi.advanceTimersByTime(300);
    expect(window.htmx.ajax.mock.calls[0][2]).toMatchObject({ target: "#calendar", select: "#calendar" });
  });

  it("does nothing on pages without a task view", () => {
    document.body.innerHTML = `<form id="account"></form>`;
    pageshow(true);
    vi.advanceTimersByTime(300);
    expect(window.htmx.ajax).not.toHaveBeenCalled();
  });

  it("reloads after a back/forward cache restore only", () => {
    document.body.innerHTML = `<div id="task-list"></div>`;
    pageshow(false);
    vi.advanceTimersByTime(300);
    expect(window.htmx.ajax).not.toHaveBeenCalled();
    pageshow(true);
    vi.advanceTimersByTime(300);
    expect(window.htmx.ajax).toHaveBeenCalledTimes(1);
  });

  it("reruns a refresh requested while one is in flight", async () => {
    document.body.innerHTML = `<div id="task-list"></div>`;
    let finish;
    window.htmx.ajax.mockImplementationOnce(() => new Promise((r) => (finish = r)));
    pageshow(true);
    vi.advanceTimersByTime(300);
    pageshow(true);
    vi.advanceTimersByTime(300);
    expect(window.htmx.ajax).toHaveBeenCalledTimes(1);
    finish();
    await vi.runAllTimersAsync();
    expect(window.htmx.ajax).toHaveBeenCalledTimes(2);
  });
});

describe("expired session", () => {
  function beforeSwap(responseURL) {
    const detail = { xhr: { responseURL }, shouldSwap: true };
    document.dispatchEvent(new window.CustomEvent("htmx:beforeSwap", { detail }));
    return detail;
  }

  it("leaves normal responses alone", () => {
    expect(beforeSwap("http://localhost/?filter=pending").shouldSwap).toBe(true);
  });

  it("does not swap a redirect to the login page", () => {
    const assign = vi.fn();
    vi.stubGlobal("location", { ...window.location, assign });
    try {
      expect(beforeSwap("http://localhost/login").shouldSwap).toBe(false);
      expect(assign).toHaveBeenCalledWith("/login");
    } finally {
      vi.unstubAllGlobals();
    }
  });
});
