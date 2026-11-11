// login.js runs the login page as on the gemini_pwd site: Log in stays
// disabled until both fields are filled, and after too many failed
// attempts a countdown (seconds from the server, in the cooldown box's
// data-seconds) keeps it disabled until the wait is over. The server
// enforces the wait either way; this only saves pointless attempts, which
// would restart it.
(function () {
  "use strict";
  const form = document.getElementById("login-form");
  const button = document.getElementById("sign-in");
  const box = document.getElementById("cooldown");
  const timer = document.getElementById("cooldown-timer");
  const text = document.getElementById("cooldown-text");
  let left = box ? parseInt(box.dataset.seconds, 10) || 0 : 0;

  // update enables Log in when both fields have text and no cooldown runs.
  function update() {
    const filled = form.username.value.trim() !== "" && form.password.value !== "";
    button.disabled = !filled || left > 0;
  }

  // mmss formats seconds as m:ss.
  function mmss(s) {
    return Math.floor(s / 60) + ":" + String(s % 60).padStart(2, "0");
  }

  // tick shows the time left and ends the cooldown when it reaches zero.
  function tick() {
    if (left <= 0) {
      box.remove();
      update();
      return;
    }
    timer.textContent = mmss(left);
    text.textContent = left > 60 ? "Too many failed attempts. Please wait:" : "Rate limited. Please wait:";
    left--;
    setTimeout(tick, 1000);
  }

  form.username.addEventListener("input", update);
  form.password.addEventListener("input", update);
  update();
  if (box) tick();
})();
