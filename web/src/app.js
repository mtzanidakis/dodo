// dodo web client: theme control + websocket live updates.
// Vanilla JS; htmx and Alpine are loaded globally from vendor.js.
(function () {
  "use strict";

  var THEME_KEY = "dodo-theme";
  var root = document.documentElement;

  // ---- theme ----------------------------------------------------------
  function applyTheme(theme) {
    root.classList.remove("dark", "light");
    if (theme === "dark") root.classList.add("dark");
    else if (theme === "light") root.classList.add("light");
    // "system" => no class; CSS prefers-color-scheme decides.
  }

  function storedTheme() {
    try {
      return localStorage.getItem(THEME_KEY);
    } catch (e) {
      return null;
    }
  }

  var THEME_ICONS = { system: "◐", light: "☀", dark: "☾" };

  function markActiveTheme(theme) {
    document.querySelectorAll("[data-set-theme]").forEach(function (el) {
      el.classList.toggle("active", el.getAttribute("data-set-theme") === theme);
    });
    document.querySelectorAll("[data-theme-icon]").forEach(function (el) {
      el.textContent = THEME_ICONS[theme] || THEME_ICONS.system;
    });
  }

  function setTheme(theme) {
    try {
      localStorage.setItem(THEME_KEY, theme);
    } catch (e) {}
    applyTheme(theme);
    markActiveTheme(theme);
  }

  // The current theme is the client-side preference (localStorage), else the
  // server-rendered <html> class, else "system". A client preference overrides
  // the server so switching feels instant and survives navigation.
  function currentTheme() {
    var pref = storedTheme();
    if (pref) return pref;
    if (root.classList.contains("dark")) return "dark";
    if (root.classList.contains("light")) return "light";
    return "system";
  }

  var pref = storedTheme();
  if (pref) applyTheme(pref);
  markActiveTheme(currentTheme());

  window.dodoSetTheme = setTheme;

  document.addEventListener("click", function (e) {
    var el = e.target.closest ? e.target.closest("[data-set-theme]") : null;
    if (el) {
      setTheme(el.getAttribute("data-set-theme"));
      var d = el.closest("details.dropdown");
      if (d) d.removeAttribute("open");
    }
  });

  // Close any open dropdown when clicking outside it.
  document.addEventListener("click", function (e) {
    document.querySelectorAll("details.dropdown[open]").forEach(function (d) {
      if (!d.contains(e.target)) d.removeAttribute("open");
    });
  });

  // ---- toasts ---------------------------------------------------------
  function toast(msg) {
    var box = document.getElementById("toasts");
    if (!box) return;
    var el = document.createElement("div");
    el.className = "toast";
    el.textContent = msg;
    box.appendChild(el);
    setTimeout(function () {
      el.style.opacity = "0";
      setTimeout(function () {
        el.remove();
      }, 250);
    }, 3500);
  }

  // ---- browser notifications ------------------------------------------
  function notifySupported() {
    return "Notification" in window;
  }

  function notify(title, body, tag) {
    if (!notifySupported() || Notification.permission !== "granted") return;
    try {
      var n = new Notification(title, { body: body, tag: tag });
      n.onclick = function () {
        window.focus();
        n.close();
      };
    } catch (e) {}
  }

  function updateNotifyUI() {
    var status = document.querySelector("[data-notify-status]");
    var btn = document.querySelector("[data-notify-enable]");
    if (!status && !btn) return;
    var state = !notifySupported() ? "unsupported" : Notification.permission;
    if (status) status.setAttribute("data-state", state);
    if (btn) btn.hidden = state !== "default";
  }

  document.addEventListener("click", function (e) {
    var el = e.target.closest ? e.target.closest("[data-notify-enable]") : null;
    if (el && notifySupported()) {
      Notification.requestPermission().then(updateNotifyUI);
    }
  });

  updateNotifyUI();

  // ---- live list refresh (debounced) ----------------------------------
  var refreshTimer = null;
  function refreshList() {
    var list = document.getElementById("task-list");
    if (!list || !window.htmx) return;
    if (refreshTimer) clearTimeout(refreshTimer);
    refreshTimer = setTimeout(function () {
      window.htmx.ajax("GET", window.location.pathname + window.location.search, {
        target: "#task-list",
        select: "#task-list",
        swap: "outerHTML",
      });
    }, 250);
  }

  // ---- websocket ------------------------------------------------------
  function wsDot(state) {
    var d = document.getElementById("ws-dot");
    if (!d) return;
    d.classList.remove("on", "off");
    d.classList.add(state);
  }

  var backoff = 1000;
  function connectWS() {
    var proto = location.protocol === "https:" ? "wss:" : "ws:";
    var ws;
    try {
      ws = new WebSocket(proto + "//" + location.host + "/ws");
    } catch (e) {
      scheduleReconnect();
      return;
    }
    ws.onopen = function () {
      backoff = 1000;
      wsDot("on");
    };
    ws.onmessage = function (ev) {
      var evt;
      try {
        evt = JSON.parse(ev.data);
      } catch (e) {
        return;
      }
      if (!evt || !evt.type) return;
      switch (evt.type) {
        case "task.created":
        case "task.updated":
        case "task.completed":
        case "task.deleted":
          refreshList();
          break;
        case "task.due":
          refreshList();
          if (evt.payload && evt.payload.title) {
            toast("⏰ " + evt.payload.title);
            notify(evt.payload.title, "Due now", "task-" + (evt.payload.id || ""));
          }
          break;
        default:
          break;
      }
    };
    ws.onclose = function () {
      wsDot("off");
      scheduleReconnect();
    };
    ws.onerror = function () {
      try {
        ws.close();
      } catch (e) {}
    };
  }

  function scheduleReconnect() {
    setTimeout(connectWS, backoff);
    backoff = Math.min(backoff * 2, 30000);
  }

  // Connect the live-updates websocket on authenticated pages (the user menu
  // only renders when logged in).
  if (document.querySelector(".usermenu")) {
    connectWS();
  }
})();
