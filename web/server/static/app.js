// app.js starts the OfficeLite WebAssembly client in the app named by
// this script tag's data-app ("word" or "excel"), as the desktop
// command line's --word / --excel would. With ?open=PATH in the address
// (the Files page's links; "Reports/Budget.xlsx" for one in a folder) it
// opens the user's cloud document PATH instead, as /server/PATH
// (serverfs.js), in whichever app the file's
// type calls for. With ?local=ID it was opened by another tab for a file
// from the user's computer, and asks that tab for the file's contents
// (serverfs.js, openDataTab), which the client opens as the Open dialog
// would. A separate file, not an inline script, so the
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

  // HANDOFF_MS is how long to wait for a file from the tab that opened
  // this one.
  const HANDOFF_MS = 30000;

  // receive asks the tab that opened this one for file id and resolves
  // with its contents (a Uint8Array).
  function receive(id) {
    return new Promise(function (resolve, reject) {
      const from = window.opener;
      if (!from) {
        reject(new Error("the tab with the file is gone"));
        return;
      }
      const timer = setTimeout(function () {
        reject(new Error("the file didn't come from the other tab"));
      }, HANDOFF_MS);
      window.addEventListener("message", function (e) {
        const m = e.data;
        if (e.origin !== location.origin || e.source !== from || !m || m.type !== "officeweb-file" || m.id !== id) return;
        clearTimeout(timer);
        resolve(m.data);
      });
      from.postMessage({ type: "officeweb-ready", id: id }, location.origin);
    });
  }

  const go = new Go();
  const params = new URLSearchParams(location.search);
  const open = params.get("open");
  const local = params.get("local");
  // The query has done its job: reloading the page starts the app empty
  // rather than reopening a file that may have changed.
  if (open !== null || local !== null) history.replaceState(null, "", location.pathname);
  go.argv = open ? ["officelite", "/server/" + open] : ["officelite", "--" + app];
  const file = local ? receive(local) : Promise.resolve(null);
  Promise.all([
    WebAssembly.instantiateStreaming(fetch("/client/officelite.wasm", { credentials: "same-origin" }), go.importObject),
    file,
  ]).then(function (got) {
    // Read by the client at start (cmd/officelite/page_js.go, localFile).
    if (got[1]) globalThis.officewebLocalFile = got[1];
    status.remove();
    return go.run(got[0].instance);
  }).catch(fail);
})();
