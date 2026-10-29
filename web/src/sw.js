// dodo service worker. It caches nothing, so the app is never stale: every
// request still goes to the network. It exists to make the app installable
// and to show a retry page instead of the browser's error when a navigation
// fails offline.
(function () {
  "use strict";

  var OFFLINE_TEXT = {
    en: { title: "Can't reach dodo", body: "Check your connection and try again.", retry: "Retry" },
    el: { title: "Δεν υπάρχει σύνδεση με το dodo", body: "Ελέγξτε τη σύνδεσή σας και δοκιμάστε ξανά.", retry: "Δοκιμή ξανά" },
  };

  function offlinePage() {
    var lang = (self.navigator.language || "en").slice(0, 2);
    var t = OFFLINE_TEXT[lang] || OFFLINE_TEXT.en;
    var html =
      '<!doctype html><html lang="' + lang + '"><head><meta charset="utf-8">' +
      '<meta name="viewport" content="width=device-width, initial-scale=1">' +
      '<meta name="color-scheme" content="light dark"><title>dodo</title>' +
      "<style>body{font-family:system-ui,sans-serif;display:grid;place-items:center;" +
      "min-height:100vh;margin:0;text-align:center}button{font:inherit;padding:.5rem 1.2rem}</style>" +
      "</head><body><main><h1>" + t.title + "</h1><p>" + t.body + "</p>" +
      '<button onclick="location.reload()">' + t.retry + "</button></main>" +
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
