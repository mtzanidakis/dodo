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

  // ---- date picker ----------------------------------------------------
  //
  // Progressive enhancement for inputs carrying data-datepicker="<pattern>".
  // A native date picker always renders in the browser's locale, so when the
  // user has chosen a date format the server sends a text box instead and we
  // attach this calendar. The text box stays authoritative: typing still
  // works and the server parses the submitted string either way, so a parse
  // miss here can only mean the wrong day is highlighted.

  // dateformat.js publishes this global and is loaded before us.
  var DF = window.dodoDateFormat;
  function dpPad(n) { return DF.pad(n); }
  function dpFormat(d, pattern) { return DF.format(d, pattern); }
  function dpParse(s, pattern) { return DF.parse(s, pattern); }
  function dpMonthName(i) { return DF.MONTHS[i]; }

  function dpSameDay(a, b) {
    return a.getFullYear() === b.getFullYear() && a.getMonth() === b.getMonth() && a.getDate() === b.getDate();
  }

  var dpOpen = null; // { input, pattern, popup, view, sel, hasTime }

  function dpClose() {
    if (!dpOpen) return;
    dpOpen.popup.remove();
    dpOpen = null;
  }

  function dpPosition() {
    if (!dpOpen) return;
    var r = dpOpen.input.getBoundingClientRect();
    var p = dpOpen.popup;
    p.style.left = window.scrollX + r.left + "px";
    // Flip above the field when there is not enough room below it.
    var below = window.innerHeight - r.bottom;
    if (below < p.offsetHeight + 8 && r.top > p.offsetHeight + 8) {
      p.style.top = window.scrollY + r.top - p.offsetHeight - 6 + "px";
    } else {
      p.style.top = window.scrollY + r.bottom + 6 + "px";
    }
  }

  function dpButton(cls, label, attrs) {
    var b = document.createElement("button");
    b.type = "button"; // never submit the surrounding form
    b.className = cls;
    b.textContent = label;
    for (var k in attrs || {}) b.setAttribute(k, attrs[k]);
    return b;
  }

  function dpRender() {
    if (!dpOpen) return;
    var st = dpOpen;
    var body = st.popup.querySelector("[data-dp-body]");
    var title = st.popup.querySelector("[data-dp-month]");
    title.textContent = dpMonthName(st.view.getMonth()) + " " + st.view.getFullYear();
    body.textContent = "";

    st.dow.forEach(function (name) {
      var el = document.createElement("div");
      el.className = "dp-dow";
      el.textContent = name;
      body.appendChild(el);
    });

    var first = new Date(st.view.getFullYear(), st.view.getMonth(), 1);
    // Monday-first offset, matching the calendar view.
    var lead = (first.getDay() + 6) % 7;
    var days = new Date(st.view.getFullYear(), st.view.getMonth() + 1, 0).getDate();
    var today = new Date();
    for (var i = 0; i < lead; i++) body.appendChild(document.createElement("div"));
    for (var day = 1; day <= days; day++) {
      var d = new Date(st.view.getFullYear(), st.view.getMonth(), day);
      var b = dpButton("dp-day", String(day), { "data-dp-day": String(day) });
      if (dpSameDay(d, today)) b.classList.add("today");
      if (st.sel && dpSameDay(d, st.sel)) {
        b.classList.add("selected");
        b.setAttribute("aria-current", "date");
      }
      body.appendChild(b);
    }
  }

  // dpCommit writes the picker's date (and time, when the pattern has one)
  // back into the text box.
  function dpCommit() {
    if (!dpOpen || !dpOpen.sel) return;
    var st = dpOpen;
    var d = new Date(st.sel.getTime());
    if (st.hasTime) {
      var t = st.popup.querySelector("[data-dp-time]");
      var parts = (t && t.value ? t.value : "09:00").split(":");
      d.setHours(parseInt(parts[0], 10) || 0, parseInt(parts[1], 10) || 0, 0, 0);
    }
    st.input.value = dpFormat(d, st.pattern);
    st.input.dispatchEvent(new Event("input", { bubbles: true }));
    st.input.dispatchEvent(new Event("change", { bubbles: true }));
  }

  function dpBuild(input) {
    var pattern = input.getAttribute("data-datepicker");
    var hasTime = DF.hasTime(pattern);
    var current = dpParse(input.value, pattern);
    var base = current || new Date();

    var popup = document.createElement("div");
    popup.className = "dp";
    popup.setAttribute("role", "dialog");
    popup.setAttribute("aria-label", "Date picker");

    var head = document.createElement("div");
    head.className = "dp-head";
    head.appendChild(dpButton("dp-nav", "‹", { "data-dp-prev": "", "aria-label": "Previous month" }));
    var month = document.createElement("div");
    month.className = "dp-month";
    month.setAttribute("data-dp-month", "");
    head.appendChild(month);
    head.appendChild(dpButton("dp-nav", "›", { "data-dp-next": "", "aria-label": "Next month" }));
    popup.appendChild(head);

    var body = document.createElement("div");
    body.className = "dp-grid";
    body.setAttribute("data-dp-body", "");
    popup.appendChild(body);

    var foot = document.createElement("div");
    foot.className = "dp-foot";
    if (hasTime) {
      var time = document.createElement("input");
      time.type = "time";
      time.className = "dp-time";
      time.setAttribute("data-dp-time", "");
      time.value = current ? dpPad(current.getHours()) + ":" + dpPad(current.getMinutes()) : "09:00";
      foot.appendChild(time);
    }
    foot.appendChild(dpButton("btn btn-sm btn-ghost", input.getAttribute("data-dp-today") || "Today", { "data-dp-todaybtn": "" }));
    foot.appendChild(dpButton("btn btn-sm btn-primary", input.getAttribute("data-dp-done") || "OK", { "data-dp-done": "" }));
    popup.appendChild(foot);

    document.body.appendChild(popup);
    dpOpen = {
      input: input,
      pattern: pattern,
      popup: popup,
      hasTime: hasTime,
      view: new Date(base.getFullYear(), base.getMonth(), 1),
      sel: current,
      dow: (input.getAttribute("data-dow") || "Mon,Tue,Wed,Thu,Fri,Sat,Sun").split(","),
    };
    dpRender();
    dpPosition();
  }

  function dpMove(days) {
    if (!dpOpen) return;
    var from = dpOpen.sel || new Date();
    var d = new Date(from.getFullYear(), from.getMonth(), from.getDate() + days);
    dpOpen.sel = d;
    dpOpen.view = new Date(d.getFullYear(), d.getMonth(), 1);
    dpRender();
  }

  document.addEventListener("click", function (e) {
    var input = e.target.closest ? e.target.closest("input[data-datepicker]") : null;
    if (input && input.getAttribute("data-datepicker")) {
      if (!dpOpen || dpOpen.input !== input) {
        dpClose();
        dpBuild(input);
      }
      return;
    }
    if (!dpOpen) return;
    if (!dpOpen.popup.contains(e.target)) {
      dpClose();
      return;
    }

    var t = e.target;
    if (t.closest("[data-dp-prev]")) {
      dpOpen.view = new Date(dpOpen.view.getFullYear(), dpOpen.view.getMonth() - 1, 1);
      dpRender();
    } else if (t.closest("[data-dp-next]")) {
      dpOpen.view = new Date(dpOpen.view.getFullYear(), dpOpen.view.getMonth() + 1, 1);
      dpRender();
    } else if (t.closest("[data-dp-todaybtn]")) {
      var now = new Date();
      dpOpen.sel = now;
      dpOpen.view = new Date(now.getFullYear(), now.getMonth(), 1);
      dpRender();
    } else if (t.closest("[data-dp-done]")) {
      dpCommit();
      dpClose();
    } else {
      var dayBtn = t.closest("[data-dp-day]");
      if (dayBtn) {
        dpOpen.sel = new Date(
          dpOpen.view.getFullYear(),
          dpOpen.view.getMonth(),
          parseInt(dayBtn.getAttribute("data-dp-day"), 10),
        );
        dpRender();
        dpCommit();
      }
    }
  });

  document.addEventListener("keydown", function (e) {
    if (!dpOpen) return;
    switch (e.key) {
      case "Escape":
        e.preventDefault();
        dpClose();
        break;
      case "ArrowLeft":  e.preventDefault(); dpMove(-1); break;
      case "ArrowRight": e.preventDefault(); dpMove(1); break;
      case "ArrowUp":    e.preventDefault(); dpMove(-7); break;
      case "ArrowDown":  e.preventDefault(); dpMove(7); break;
      case "Enter":
        // Enter picks the highlighted day rather than submitting the form.
        if (dpOpen.sel) {
          e.preventDefault();
          dpCommit();
          dpClose();
        }
        break;
      default:
        break;
    }
  });

  // Typing in the field keeps the calendar in sync without stealing the input.
  document.addEventListener("input", function (e) {
    if (!dpOpen || e.target !== dpOpen.input) return;
    var d = dpParse(dpOpen.input.value, dpOpen.pattern);
    if (!d) return;
    dpOpen.sel = d;
    dpOpen.view = new Date(d.getFullYear(), d.getMonth(), 1);
    // Follow the typed time too, otherwise confirming the calendar would
    // overwrite it with whatever the field held when the popup opened.
    if (dpOpen.hasTime) {
      var t = dpOpen.popup.querySelector("[data-dp-time]");
      if (t) t.value = dpPad(d.getHours()) + ":" + dpPad(d.getMinutes());
    }
    dpRender();
  });

  window.addEventListener("resize", dpPosition);
  window.addEventListener("scroll", dpPosition, true);

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
