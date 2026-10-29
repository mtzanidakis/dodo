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
NotificationMock.permission = "granted";

const showNotification = vi.fn(() => Promise.resolve());
let registration;

beforeAll(async () => {
  globalThis.WebSocket = FakeWebSocket;
  globalThis.Notification = NotificationMock;
  Object.defineProperty(navigator, "serviceWorker", {
    value: {
      register: () => Promise.resolve(),
      getRegistration: () => Promise.resolve(registration),
    },
    configurable: true,
  });
  window.htmx = { ajax: vi.fn(() => Promise.resolve()) };
  document.body.innerHTML = `<details class="usermenu"></details>`;
  await import("./dateformat.js");
  await import("./app.js");
  FakeWebSocket.last.readyState = FakeWebSocket.OPEN;
  FakeWebSocket.last.onopen();
});

beforeEach(() => {
  NotificationMock.mockClear();
  NotificationMock.permission = "granted";
  showNotification.mockClear();
  registration = { active: {}, showNotification };
});

async function taskDue(title) {
  FakeWebSocket.last.onmessage({ data: JSON.stringify({ type: "task.due", payload: { id: "t1", title } }) });
  await new Promise((r) => setTimeout(r, 0));
}

describe("due-task notifications", () => {
  it("are shown through the service worker", async () => {
    await taskDue("Pay bill");
    expect(showNotification).toHaveBeenCalledWith("Pay bill", expect.objectContaining({ tag: "task-t1" }));
    expect(NotificationMock).not.toHaveBeenCalled();
  });

  it("fall back to the constructor without an active worker", async () => {
    registration = undefined;
    await taskDue("Pay bill");
    expect(NotificationMock).toHaveBeenCalledWith("Pay bill", expect.objectContaining({ tag: "task-t1" }));
  });

  it("fall back to the constructor when the worker refuses", async () => {
    showNotification.mockImplementationOnce(() => Promise.reject(new TypeError("no")));
    await taskDue("Pay bill");
    expect(NotificationMock).toHaveBeenCalledTimes(1);
  });

  it("are not shown without permission", async () => {
    NotificationMock.permission = "default";
    await taskDue("Pay bill");
    expect(showNotification).not.toHaveBeenCalled();
    expect(NotificationMock).not.toHaveBeenCalled();
  });
});
