// files.js adds drag and drop to the Files page (templates/files.html):
// with a mouse and room (officeweb.explorerLayout, static/serverfs.js) a
// document or folder can be dragged by its name onto a folder in the
// list or in the path at the top, and moves there. Phones and tablets
// keep the Move form. A separate file, not an inline script, so the
// Content-Security-Policy can forbid inline scripts.
(function () {
  "use strict";
  const ow = globalThis.officeweb;
  const table = document.querySelector("[data-dir]");
  if (!ow || !ow.explorerLayout || !ow.explorerLayout() || !table) return;
  const dir = table.dataset.dir;

  // reload shows this folder again, saying the move worked.
  function reload() {
    const q = new URLSearchParams();
    if (dir) q.set("dir", dir);
    q.set("msg", "moved");
    location.assign("/files?" + q.toString());
  }

  document.querySelectorAll("tr[data-path]").forEach(function (row) {
    const name = row.querySelector(".fname");
    if (name) ow.dragSource(name, row.dataset.path);
  });
  document.querySelectorAll("[data-folder]").forEach(function (target) {
    ow.dropTarget(target, target.dataset.folder, reload);
  });
  const hint = document.getElementById("drag-hint");
  if (hint) hint.hidden = false;
})();
