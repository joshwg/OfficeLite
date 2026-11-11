// app.js starts the OfficeLite WebAssembly client in the app named by
// this script tag's data-app ("word" or "excel"), as the desktop
// command line's --word / --excel would. With ?open=NAME in the address
// (the Files page's links) it opens the user's server document NAME
// instead, as /server/NAME (serverfs.js), in whichever app the file's
// type calls for. A separate file, not an inline script, so the
// Content-Security-Policy can forbid inline scripts.
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
  const open = new URLSearchParams(location.search).get("open");
  // The query has done its job: reloading the page starts the app empty
  // rather than reopening a file that may have changed.
  if (open !== null) history.replaceState(null, "", location.pathname);
  go.argv = open ? ["officelite", "/server/" + open] : ["officelite", "--" + app];
  WebAssembly.instantiateStreaming(fetch("/client/officelite.wasm", { credentials: "same-origin" }), go.importObject)
    .then(function (result) {
      status.remove();
      return go.run(result.instance);
    })
    .catch(fail);
})();
