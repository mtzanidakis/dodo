import { describe, it, expect, beforeEach, afterEach, vi } from "vitest";

import "./dateformat.js";
import "./app.js";

function becomeVisible() {
  Object.defineProperty(document, "visibilityState", { value: "visible", configurable: true });
  document.dispatchEvent(new window.Event("visibilitychange"));
}

function becomeHidden() {
  Object.defineProperty(document, "visibilityState", { value: "hidden", configurable: true });
  document.dispatchEvent(new window.Event("visibilitychange"));
}

beforeEach(() => {
  vi.useFakeTimers();
  window.htmx = { ajax: vi.fn() };
});

afterEach(() => {
  vi.useRealTimers();
  delete window.htmx;
  document.body.innerHTML = "";
});

describe("refresh on resume", () => {
  it("reloads the task list when the page becomes visible", () => {
    document.body.innerHTML = `<div id="task-list"></div>`;
    becomeHidden();
    becomeVisible();
    vi.advanceTimersByTime(300);
    expect(window.htmx.ajax).toHaveBeenCalledTimes(1);
    const [method, , opts] = window.htmx.ajax.mock.calls[0];
    expect(method).toBe("GET");
    expect(opts).toMatchObject({ target: "#task-list", select: "#task-list", swap: "outerHTML" });
  });

  it("reloads the calendar in calendar view", () => {
    document.body.innerHTML = `<div id="calendar"></div>`;
    becomeVisible();
    vi.advanceTimersByTime(300);
    expect(window.htmx.ajax.mock.calls[0][2]).toMatchObject({ target: "#calendar", select: "#calendar" });
  });

  it("does nothing while hidden or on pages without a task view", () => {
    document.body.innerHTML = `<div id="task-list"></div>`;
    becomeHidden();
    vi.advanceTimersByTime(300);
    document.body.innerHTML = `<form id="account"></form>`;
    becomeVisible();
    vi.advanceTimersByTime(300);
    expect(window.htmx.ajax).not.toHaveBeenCalled();
  });

  it("reloads after a back/forward cache restore", () => {
    document.body.innerHTML = `<div id="task-list"></div>`;
    const ev = new window.Event("pageshow");
    ev.persisted = true;
    window.dispatchEvent(ev);
    vi.advanceTimersByTime(300);
    expect(window.htmx.ajax).toHaveBeenCalledTimes(1);
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
