import { describe, it, expect, beforeAll, beforeEach, vi } from "vitest";

// app.js only opens the websocket when the logged-in user menu is present at
// load, so the page and a fake WebSocket are set up before importing it. The
// tests run in order against that one app.js instance.
class FakeWebSocket {
  static CONNECTING = 0;
  static OPEN = 1;
  static CLOSING = 2;
  static CLOSED = 3;
  static instances = [];

  constructor(url) {
    this.url = url;
    this.readyState = FakeWebSocket.CONNECTING;
    FakeWebSocket.instances.push(this);
  }

  open() {
    this.readyState = FakeWebSocket.OPEN;
    this.onopen && this.onopen();
  }

  message(obj) {
    this.onmessage && this.onmessage({ data: JSON.stringify(obj) });
  }

  close() {
    if (this.readyState < FakeWebSocket.CLOSING) this.readyState = FakeWebSocket.CLOSING;
  }

  finishClose() {
    this.readyState = FakeWebSocket.CLOSED;
    this.onclose && this.onclose();
  }
}

const sockets = FakeWebSocket.instances;
const latest = () => sockets[sockets.length - 1];

function setVisibility(state) {
  Object.defineProperty(document, "visibilityState", { value: state, configurable: true });
  document.dispatchEvent(new window.Event("visibilitychange"));
}

function quickResume(awayMs = 1000) {
  setVisibility("hidden");
  vi.advanceTimersByTime(awayMs);
  setVisibility("visible");
}

beforeAll(async () => {
  vi.useFakeTimers();
  globalThis.WebSocket = FakeWebSocket;
  window.htmx = { ajax: vi.fn(() => Promise.resolve()) };
  document.body.innerHTML = `<details class="usermenu"></details><div id="task-list"></div>`;
  await import("./dateformat.js");
  await import("./app.js");
});

beforeEach(() => {
  window.htmx.ajax.mockClear();
});

describe("websocket lifecycle", () => {
  it("connects on load", () => {
    expect(sockets).toHaveLength(1);
    expect(sockets[0].url).toMatch(/\/ws$/);
    sockets[0].open();
  });

  it("refreshes on task events", () => {
    latest().message({ type: "task.created" });
    vi.advanceTimersByTime(300);
    expect(window.htmx.ajax).toHaveBeenCalledTimes(1);
  });

  it("does not touch a healthy socket on a quick resume", () => {
    quickResume();
    vi.advanceTimersByTime(300);
    expect(sockets).toHaveLength(1);
    expect(window.htmx.ajax).not.toHaveBeenCalled();
  });

  it("reconnects after the backoff and catches up on reopen", () => {
    sockets[0].finishClose();
    vi.advanceTimersByTime(999);
    expect(sockets).toHaveLength(1);
    vi.advanceTimersByTime(1);
    expect(sockets).toHaveLength(2);
    latest().open();
    vi.advanceTimersByTime(300);
    expect(window.htmx.ajax).toHaveBeenCalledTimes(1);
  });

  it("stays connected while heartbeats arrive", () => {
    for (let i = 0; i < 6; i++) {
      vi.advanceTimersByTime(30000);
      latest().message({ type: "ping" });
    }
    expect(sockets).toHaveLength(2);
  });

  it("replaces a socket that went silent while still OPEN", () => {
    const stale = latest();
    vi.advanceTimersByTime(90000);
    expect(sockets).toHaveLength(3);
    expect(stale.readyState).toBe(FakeWebSocket.CLOSING);
    // The replaced socket's late close must not schedule another one.
    stale.finishClose();
    vi.advanceTimersByTime(60000);
    expect(sockets).toHaveLength(3);
    latest().open();
  });

  it("reconnects at once on resume instead of waiting out the backoff", () => {
    latest().finishClose();
    expect(sockets).toHaveLength(3);
    window.htmx.ajax.mockClear();
    // Back before the 1s backoff timer fires, so it is still pending.
    quickResume(100);
    expect(sockets).toHaveLength(4);
    vi.advanceTimersByTime(300);
    expect(window.htmx.ajax).toHaveBeenCalledTimes(1);
    // The pending backoff timer was cancelled.
    vi.advanceTimersByTime(60000);
    expect(sockets).toHaveLength(4);
  });

  it("leaves a connecting socket alone on resume", () => {
    expect(latest().readyState).toBe(FakeWebSocket.CONNECTING);
    quickResume();
    expect(sockets).toHaveLength(4);
  });
});
