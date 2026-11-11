// app.js starts the OfficeLite WebAssembly client in the app named by
// this script tag's data-app ("word" or "excel"), as the desktop
// command line's --word / --excel would. A separate file, not an inline
// script, so the Content-Security-Policy can forbid inline scripts.
(function () {
  "use strict";
  const app = document.currentScript.dataset.app;
  const status = document.getElementById("status");

  // fail shows why the client didn't start.
  function fail(err) {
    status.textContent = "Couldn't start OfficeLite: " + err;
    status.classList.add("err");
  }

  const go = new Go();
  go.argv = ["officelite", "--" + app];
  WebAssembly.instantiateStreaming(fetch("/client/officelite.wasm", { credentials: "same-origin" }), go.importObject)
    .then(function (result) {
      status.remove();
      return go.run(result.instance);
    })
    .catch(fail);
})();
