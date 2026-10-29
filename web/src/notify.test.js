import { describe, it, expect, beforeAll, beforeEach, vi } from "vitest";

// Notifications fire on a "task.due" websocket event, so app.js is loaded on
// a signed-in page with a fake WebSocket and a stubbed Notification API.
class FakeWebSocket {
  static CONNECTING = 0;
  static OPEN = 1;
  static CLOSING = 2;
  static CLOSED = 3;
  static last = null;
  constructor() {
    this.readyState = FakeWebSocket.CONNECTING;
    FakeWebSocket.last = this;
  }
  close() {}
}

const NotificationMock = vi.fn();
const showNotification = vi.fn(() => Promise.resolve());
let shown;
let registration;

const serviceWorker = {
  register: () => Promise.resolve(),
  getRegistration: () => Promise.resolve(registration),
};

beforeAll(async () => {
  globalThis.WebSocket = FakeWebSocket;
  globalThis.Notification = NotificationMock;
  Object.defineProperty(navigator, "serviceWorker", { value: serviceWorker, configurable: true });
  window.htmx = { ajax: vi.fn(() => Promise.resolve()) };
  document.body.innerHTML = `<details class="usermenu"></details>`;
  document.body.setAttribute("data-notify-due", "Λήγει τώρα");
  await import("./dateformat.js");
  await import("./app.js");
  FakeWebSocket.last.readyState = FakeWebSocket.OPEN;
  FakeWebSocket.last.onopen();
});

beforeEach(() => {
  NotificationMock.mockClear();
  NotificationMock.permission = "granted";
  showNotification.mockClear();
  shown = [];
  registration = {
    active: {},
    showNotification,
    getNotifications: ({ tag }) => Promise.resolve(shown.filter((n) => n.tag === tag)),
  };
  Object.defineProperty(navigator, "serviceWorker", { value: serviceWorker, configurable: true });
});

async function taskDue(payload) {
  FakeWebSocket.last.onmessage({ data: JSON.stringify({ type: "task.due", payload }) });
  await new Promise((r) => setTimeout(r, 0));
}

const due = { id: "t1", title: "Pay bill", notified_at: "2026-10-29T17:00:00Z" };

describe("due-task notifications", () => {
  it("are shown through the service worker, localized and re-alerting", async () => {
    await taskDue(due);
    expect(showNotification).toHaveBeenCalledWith("Pay bill", {
      body: "Λήγει τώρα",
      tag: "task-t1",
      renotify: true,
      icon: "/icons/icon-192.png",
      data: { at: "2026-10-29T17:00:00Z" },
    });
    expect(NotificationMock).not.toHaveBeenCalled();
  });

  it("are shown once when several tabs receive the same reminder", async () => {
    shown = [{ tag: "task-t1", data: { at: due.notified_at } }];
    await taskDue(due);
    expect(showNotification).not.toHaveBeenCalled();
  });

  it("replace the previous reminder for the same task", async () => {
    shown = [{ tag: "task-t1", data: { at: "2026-10-29T16:00:00Z" } }];
    await taskDue(due);
    expect(showNotification).toHaveBeenCalledTimes(1);
  });

  it("fall back to the constructor without a registration", async () => {
    registration = undefined;
    await taskDue(due);
    expect(NotificationMock).toHaveBeenCalledWith("Pay bill", expect.objectContaining({ tag: "task-t1" }));
  });

  it("fall back to the constructor while the worker is not active", async () => {
    registration.active = null;
    await taskDue(due);
    expect(showNotification).not.toHaveBeenCalled();
    expect(NotificationMock).toHaveBeenCalledTimes(1);
  });

  it("fall back to the constructor without service worker support", async () => {
    delete navigator.serviceWorker;
    await taskDue(due);
    expect(NotificationMock).toHaveBeenCalledTimes(1);
  });

  it("fall back to the constructor when the worker refuses", async () => {
    showNotification.mockImplementationOnce(() => Promise.reject(new TypeError("no")));
    await taskDue(due);
    expect(NotificationMock).toHaveBeenCalledTimes(1);
  });

  it("are not shown without permission", async () => {
    NotificationMock.permission = "default";
    await taskDue(due);
    expect(showNotification).not.toHaveBeenCalled();
    expect(NotificationMock).not.toHaveBeenCalled();
  });
});
