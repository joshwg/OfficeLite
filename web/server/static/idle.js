// idle.js logs out a user who has left the site alone: after data-idle
// seconds without a key press, click, touch or mouse movement in any of
// their tabs (shared through localStorage), the page logs out and goes to
// the login page, which brings them back here afterwards. data-warn
// seconds before that it shows a countdown with a Stay logged in button.
// While the user is busy it tells the server so every data-ping seconds
// (GET /api/ping), because typing in WordLite or ExcelLite sends the
// server nothing and the session would otherwise end under them. The
// server ends an idle session on its own too; this makes the page follow.
// Loaded by layout.html (when logged in) and app.html.
(function () {
  "use strict";
  const d = document.currentScript.dataset;
  const IDLE = parseInt(d.idle, 10) * 1000;
  const WARN = parseInt(d.warn, 10) * 1000;
  const PING = parseInt(d.ping, 10) * 1000;
  const KEY = "officeweb.lastActive";
  let last = Date.now();
  let lastPing = Date.now();
  let leaving = false;
  let bar = null;

  // lastActive is the latest activity in any of the user's tabs.
  function lastActive() {
    try {
      return Math.max(last, parseInt(localStorage.getItem(KEY), 10) || 0);
    } catch (e) {
      return last; // storage blocked: this tab only
    }
  }

  // active records activity (at most once a second) and hides the warning.
  function active() {
    const now = Date.now();
    if (now - last < 1000) return;
    last = now;
    try { localStorage.setItem(KEY, String(now)); } catch (e) { /* this tab only */ }
    if (bar) bar.hidden = true;
  }

  // toLogin goes to the login page, coming back to this page afterwards.
  function toLogin() {
    location.href = "/login?next=" + encodeURIComponent(location.pathname + location.search);
  }

  // logout ends the session and goes to the login page.
  function logout() {
    leaving = true;
    fetch("/logout", { method: "POST", credentials: "same-origin" }).then(toLogin, toLogin);
  }

  // ping keeps the server's session alive; a session already gone sends
  // the user to log in again.
  function ping() {
    lastPing = Date.now();
    fetch("/api/ping", { credentials: "same-origin", cache: "no-store" }).then(function (r) {
      if (r.status === 401 && !leaving) {
        leaving = true;
        toLogin();
      }
    }, function () { /* offline: try again next time */ });
  }

  // warn shows (or updates) the countdown bar, ms before the logout.
  function warn(ms) {
    if (!bar) {
      bar = document.createElement("div");
      bar.className = "idle-bar";
      bar.setAttribute("role", "alert");
      const text = document.createElement("span");
      const stay = document.createElement("button");
      stay.type = "button";
      stay.textContent = "Stay logged in";
      stay.addEventListener("click", function () { active(); ping(); });
      bar.append(text, stay);
      document.body.appendChild(bar);
    }
    const s = Math.ceil(ms / 1000);
    bar.firstChild.textContent = "You'll be logged out in " + Math.floor(s / 60) + ":" +
      String(s % 60).padStart(2, "0") + " because you've been away. Unsaved changes will be lost.";
    bar.hidden = false;
  }

  // tick checks every second whether to ping, warn or log out.
  function tick() {
    if (leaving) return;
    const now = Date.now();
    const idle = now - lastActive();
    if (idle >= IDLE) {
      logout();
      return;
    }
    if (idle >= IDLE - WARN) {
      warn(IDLE - idle);
    } else if (bar) {
      bar.hidden = true; // active in another tab
    }
    if (now - lastPing >= PING && idle < PING) ping();
  }

  ["keydown", "pointerdown", "pointermove", "wheel", "touchstart"].forEach(function (type) {
    addEventListener(type, active, { capture: true, passive: true });
  });
  active();
  setInterval(tick, 1000);
})();
