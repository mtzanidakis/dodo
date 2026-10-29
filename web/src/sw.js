// dodo service worker. It caches nothing; it makes the app installable,
// shows due-task notifications (the only way on Android), and shows a retry
// page when a page load cannot reach the server.
(function () {
  "use strict";

  // Prepended by the server in the user's locale (internal/web/pwa.go).
  var t = self.DODO_OFFLINE;

  // What a reverse proxy answers while dodo itself is down.
  var UNREACHABLE = { 502: true, 503: true, 504: true };

  function esc(s) {
    return String(s).replace(/[&<>"]/g, function (c) {
      return { "&": "&amp;", "<": "&lt;", ">": "&gt;", '"': "&quot;" }[c];
    });
  }

  function offlinePage() {
    var html =
      '<!doctype html><html lang="' + esc(t.lang) + '"><head><meta charset="utf-8">' +
      '<meta name="viewport" content="width=device-width, initial-scale=1">' +
      '<meta name="color-scheme" content="light dark"><title>dodo</title>' +
      "<style>body{font-family:system-ui,sans-serif;display:grid;place-items:center;" +
      "min-height:100vh;margin:0;text-align:center}button{font:inherit;padding:.5rem 1.2rem}</style>" +
      "</head><body><main><h1>" + esc(t.title) + "</h1><p>" + esc(t.body) + "</p>" +
      '<button onclick="location.reload()">' + esc(t.retry) + "</button></main>" +
      '<script>addEventListener("online",function(){location.reload()})</script>' +
      "</body></html>";
    return new Response(html, { headers: { "Content-Type": "text/html; charset=utf-8" } });
  }

  self.addEventListener("install", function () {
    self.skipWaiting();
  });

  self.addEventListener("activate", function (event) {
    event.waitUntil(
      Promise.all([
        self.clients.claim(),
        self.registration.navigationPreload ? self.registration.navigationPreload.enable() : null,
      ]),
    );
  });

  // Focus a window already showing the task list; otherwise open one rather
  // than navigating a window away from a page that may hold unsaved input.
  self.addEventListener("notificationclick", function (event) {
    event.notification.close();
    var open = function () {
      return self.clients.openWindow("/");
    };
    event.waitUntil(
      self.clients.matchAll({ type: "window", includeUncontrolled: true }).then(function (wins) {
        for (var i = 0; i < wins.length; i++) {
          if (new URL(wins[i].url).pathname === "/") return wins[i].focus().catch(open);
        }
        return open();
      }),
    );
  });

  // Only GET page loads: reloading a retry page shown for a POST would
  // resubmit the form.
  self.addEventListener("fetch", function (event) {
    var req = event.request;
    if (req.mode !== "navigate" || req.method !== "GET") return;
    event.respondWith(
      Promise.resolve(event.preloadResponse)
        .then(function (preloaded) {
          return preloaded || fetch(req);
        })
        .then(function (res) {
          return UNREACHABLE[res.status] ? offlinePage() : res;
        })
        .catch(offlinePage),
    );
  });
})();
