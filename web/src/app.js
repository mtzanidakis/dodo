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

  function setTheme(theme) {
    try {
      localStorage.setItem(THEME_KEY, theme);
    } catch (e) {}
    applyTheme(theme);
    document.querySelectorAll("[data-theme-select]").forEach(function (el) {
      el.value = theme;
    });
  }

  // A client-side preference (localStorage) overrides the server-rendered
  // class so switching feels instant and survives navigation.
  var pref = storedTheme();
  if (pref) applyTheme(pref);

  window.dodoSetTheme = setTheme;

  document.addEventListener("change", function (e) {
    var el = e.target;
    if (el && el.hasAttribute && el.hasAttribute("data-theme-select")) {
      setTheme(el.value);
    }
  });

  // Close the user dropdown when clicking outside it.
  document.addEventListener("click", function (e) {
    document.querySelectorAll("details.usermenu[open]").forEach(function (d) {
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
          if (evt.payload && evt.payload.title) toast("⏰ " + evt.payload.title);
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

  if (document.getElementById("ws-dot")) {
    connectWS();
  }
})();
