// dodo service worker. It caches nothing, so the app is never stale: every
// request still goes to the network. It exists to make the app installable
// and to show a retry page instead of the browser's error when a navigation
// fails offline.
(function () {
  "use strict";

  // The server prepends self.DODO_OFFLINE with the offline-page strings in
  // the user's locale (see handleServiceWorker in internal/web/pwa.go).
  var t = self.DODO_OFFLINE;

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

  self.addEventListener("fetch", function (event) {
    if (event.request.mode !== "navigate") return;
    event.respondWith(
      Promise.resolve(event.preloadResponse)
        .then(function (preloaded) {
          return preloaded || fetch(event.request);
        })
        .catch(offlinePage),
    );
  });
})();
