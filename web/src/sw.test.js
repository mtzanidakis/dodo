import { describe, it, expect, beforeAll, beforeEach, afterEach, vi } from "vitest";

// sw.js registers its listeners on `self`; capture them instead of letting
// jsdom's window keep them, then drive the fetch handler directly.
const handlers = {};

beforeAll(async () => {
  self.DODO_OFFLINE = { lang: "el", title: "Δεν <b>", body: "Έλεγξε", retry: "Ξανά" };
  const spy = vi.spyOn(self, "addEventListener").mockImplementation((type, fn) => {
    handlers[type] = fn;
  });
  await import("./sw.js");
  spy.mockRestore();
});

let fetchMock;
beforeEach(() => {
  fetchMock = vi.fn();
  vi.stubGlobal("fetch", fetchMock);
});

afterEach(() => {
  vi.unstubAllGlobals();
});

function dispatchFetch({ mode = "navigate", method = "GET", preload } = {}) {
  let responded = null;
  handlers.fetch({
    request: { mode, method },
    preloadResponse: preload,
    respondWith: (p) => {
      responded = p;
    },
  });
  return responded;
}

async function expectOfflinePage(promise) {
  const res = await promise;
  const html = await res.text();
  expect(html).toContain('lang="el"');
  expect(html).toContain("Δεν &lt;b&gt;");
  expect(html).toContain("Ξανά");
}

describe("service worker fetch", () => {
  it("ignores subresource requests", () => {
    expect(dispatchFetch({ mode: "cors" })).toBeNull();
  });

  it("ignores POST navigations so a retry never resubmits a form", () => {
    expect(dispatchFetch({ method: "POST" })).toBeNull();
  });

  it("passes server responses through, errors included", async () => {
    for (const status of [200, 404, 500]) {
      const res = new Response("ok", { status });
      fetchMock.mockResolvedValueOnce(res);
      expect(await dispatchFetch()).toBe(res);
    }
  });

  it("uses the navigation preload response when there is one", async () => {
    const res = new Response("preloaded");
    expect(await dispatchFetch({ preload: Promise.resolve(res) })).toBe(res);
    expect(fetchMock).not.toHaveBeenCalled();
  });

  it("shows the retry page when the network fails", async () => {
    fetchMock.mockRejectedValueOnce(new TypeError("Failed to fetch"));
    await expectOfflinePage(dispatchFetch());
  });

  it("shows the retry page when a proxy reports dodo unreachable", async () => {
    for (const status of [502, 503, 504]) {
      fetchMock.mockResolvedValueOnce(new Response("bad gateway", { status }));
      await expectOfflinePage(dispatchFetch());
    }
  });
});
