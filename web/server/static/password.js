// password.js gives every password field on the page an eye button that
// shows or hides what was typed, as on the gemini_pwd site. Fields are
// hidden again when their form is submitted, so going back to the page
// doesn't show a password. Loaded by layout.html on every server page.
(function () {
  "use strict";
  const SVG = "http://www.w3.org/2000/svg";

  // Shapes of the two icons (24×24, drawn with strokes): an open eye, and
  // the same eye struck through.
  const ICONS = {
    show: [
      ["path", { d: "M1 12s4-8 11-8 11 8 11 8-4 8-11 8-11-8-11-8z" }],
      ["circle", { cx: "12", cy: "12", r: "3" }],
    ],
    hide: [
      ["path", { d: "M1 12s4-8 11-8 11 8 11 8-4 8-11 8-11-8-11-8z" }],
      ["circle", { cx: "12", cy: "12", r: "3" }],
      ["line", { x1: "3", y1: "3", x2: "21", y2: "21" }],
    ],
  };

  // icon returns a new <svg> drawing the named icon.
  function icon(name) {
    const svg = document.createElementNS(SVG, "svg");
    svg.setAttribute("viewBox", "0 0 24 24");
    svg.setAttribute("aria-hidden", "true");
    for (const [tag, attrs] of ICONS[name]) {
      const el = document.createElementNS(SVG, tag);
      for (const k in attrs) el.setAttribute(k, attrs[k]);
      svg.appendChild(el);
    }
    return svg;
  }

  // setShown shows (true) or hides the field's text and updates its button.
  function setShown(input, button, shown) {
    input.type = shown ? "text" : "password";
    button.replaceChildren(icon(shown ? "hide" : "show"));
    button.setAttribute("aria-pressed", String(shown));
    button.setAttribute("aria-label", shown ? "Hide password" : "Show password");
    button.title = shown ? "Hide password" : "Show password";
  }

  // addEye wraps a password field with an eye button.
  function addEye(input) {
    const wrap = document.createElement("span");
    wrap.className = "pw";
    input.parentNode.insertBefore(wrap, input);
    wrap.appendChild(input);
    const button = document.createElement("button");
    button.type = "button";
    button.className = "eye";
    wrap.appendChild(button);
    setShown(input, button, false);
    button.addEventListener("click", function (e) {
      e.preventDefault(); // inside a <label>: don't also focus via the label
      setShown(input, button, input.type === "password");
      input.focus();
    });
    if (input.form) {
      input.form.addEventListener("submit", function () { setShown(input, button, false); });
    }
  }

  document.querySelectorAll('input[type="password"]').forEach(addEye);
})();
