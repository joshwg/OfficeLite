// serverfs.js gives the WebAssembly client the user's server documents
// (cloud files) as files. Go's os package in the browser calls a
// Node-style globalThis.fs (wasm_exec.js puts a stub there that fails
// everything); this script, loaded first, puts a file system there
// instead in which /server is the user's top folder, its folders real
// folders, read and written through the server's /api/files, /api/folders
// and /api/move. So WordLite and ExcelLite open, save and AutoSave
// /server/Reports/Budget.xlsx with the same code that handles files on a
// desktop.
//
//   * A path's last name says what it is, as on the server: a name ending
//     in a stored extension is a document; one starting with "." or "~$",
//     or any name created for writing without such an extension (".tmp"
//     temporary files, lock files), is a file in this page's memory only;
//     any other name is a folder.
//   * A document is fetched whole when opened and kept in memory; what is
//     written goes to the server when the file is closed (or, for the
//     hidden temporary file a safe save writes first, when it is renamed
//     over the document).
//   * Every path outside /server fails with ENOSYS, as with the stub.
//
// It also offers the apps' Open and Save As dialogs of cloud files, which
// move between folders and make new ones (window.officeweb.openDialog /
// saveDialog, used by
// common/gui/filedialog_js.go), a new tab for each further file the apps
// open (officeweb.openTab, cmd/officelite/page_js.go; openDataTab for a
// file from the user's computer, common/gui/opentab_js.go), and Yes/No
// confirmation for the server
// pages' forms marked data-confirm. The file- and folder-name rules come
// from the server through this script tag's data-max-name,
// data-max-depth and data-exts (the server checks them again).
(function () {
  "use strict";
  const MOUNT = "/server";
  // HANDOFF_MS is how long a file from the user's computer waits for the
  // new tab opened for it (openDataTab).
  const HANDOFF_MS = 120000;
  // No document under Node, where serverfs_test.go runs this file.
  const hasDOM = typeof document !== "undefined";
  const script = hasDOM ? document.currentScript : null;
  // The rules from the script tag, or (under Node) from
  // globalThis.officewebRules = { maxName, maxDepth, exts }.
  const rules = script ? script.dataset : (globalThis.officewebRules || {});
  const MAX_NAME = parseInt(rules.maxName || "64", 10);
  const MAX_DEPTH = parseInt(rules.maxDepth || "4", 10);
  const EXTS = (rules.exts || "").split(" ").filter(Boolean);

  // ---- errors ----

  // The HTTP statuses of /api/files and the error codes Go maps them to.
  const CODES = { 400: "EINVAL", 401: "EACCES", 403: "EACCES", 404: "ENOENT", 409: "EEXIST", 413: "EFBIG", 507: "ENOSPC" };

  // fsError returns an Error with a Node-style code, which Go turns into
  // the matching syscall.Errno.
  function fsError(code) {
    const err = new Error(code);
    err.code = code;
    return err;
  }

  // ---- names ----

  // commonProblem returns why name (already trimmed) breaks the rules
  // file and folder names share, or "": what is "File" or "Folder".
  function commonProblem(name, what) {
    if (name === "") return "Type a " + what.toLowerCase() + " name.";
    if (name.length > MAX_NAME) return what + " names may have at most " + MAX_NAME + " characters.";
    for (let i = 0; i < name.length; i++) {
      const c = name.charCodeAt(i);
      if (c < 0x20 || c > 0x7e) return what + " names may contain only printable ASCII characters (no control, invisible or accented characters).";
    }
    if (/[\/\\*]/.test(name)) return what + " names may not contain '/', '\\' or '*'.";
    if (name.startsWith(".") || name.startsWith("~$")) return what + " names may not start with '.' or '~$'.";
    return "";
  }

  // nameProblem returns why name (already trimmed) can't be a server
  // document's name, or "" when it can: the rules of web/names.
  function nameProblem(name) {
    const why = commonProblem(name, "File");
    if (why) return why;
    if (!storable(name)) return "File names must end in one of " + EXTS.join(" ") + ".";
    return "";
  }

  // folderProblem returns why name (already trimmed) can't be a folder's
  // name, or "" when it can.
  function folderProblem(name) {
    const why = commonProblem(name, "Folder");
    if (why) return why;
    if (storable(name)) return "Folder names may not end in a file extension (" + EXTS.join(" ") + ").";
    return "";
  }

  // extOf returns name's extension in lowercase (".docx"), or "".
  function extOf(name) {
    const dot = name.lastIndexOf(".");
    return dot > 0 ? name.slice(dot).toLowerCase() : "";
  }

  // storable reports whether the server keeps files named like name.
  function storable(name) {
    return EXTS.includes(extOf(name));
  }

  // hidden reports whether name is always this page's own: a temporary
  // or lock file.
  function hidden(name) {
    return name.startsWith(".") || name.startsWith("~$");
  }

  // isDoc reports whether name is a server document's.
  function isDoc(name) {
    return !hidden(name) && storable(name);
  }

  // join joins a folder path ("" for the top) and a name.
  function join(dir, name) {
    return dir === "" ? name : dir + "/" + name;
  }

  // parse splits a path under /server into { root: true, rel: "" } for
  // /server itself, or { rel, dir, name } for something in it (rel its
  // path below /server, dir the folder holding it), or null for anything
  // else.
  function parse(path) {
    if (typeof path !== "string") return null;
    if (path === MOUNT || path === MOUNT + "/") return { root: true, rel: "", dir: "", name: "" };
    if (!path.startsWith(MOUNT + "/")) return null;
    let rel = path.slice(MOUNT.length + 1);
    if (rel.endsWith("/")) rel = rel.slice(0, -1);
    const parts = rel.split("/");
    if (parts.some(function (p) { return p === "" || p === "." || p === ".."; })) return null;
    const name = parts.pop();
    return { rel: rel, dir: parts.join("/"), name: name };
  }

  // ---- the server ----

  // encodePath escapes each name of a '/'-joined path for a URL.
  function encodePath(rel) {
    return rel.split("/").map(encodeURIComponent).join("/");
  }

  // api calls url with method and body and returns the response, or
  // throws an fsError (after telling the user what the server said,
  // except for a GET's 404 and the statuses in quiet).
  async function api(method, url, body, quiet) {
    const init = { method: method, credentials: "same-origin", cache: "no-store" };
    if (body !== undefined) init.body = body;
    let resp;
    try {
      resp = await fetch(url, init);
    } catch (e) {
      toast("Couldn't reach the server: " + e.message);
      throw fsError("EIO");
    }
    if (resp.ok) return resp;
    let msg = resp.statusText, code = "";
    try {
      const b = await resp.json();
      msg = b.error || msg;
      code = b.code || "";
    } catch (e) { /* not JSON */ }
    if (resp.status === 401) msg = "You've been logged out. Log in again in another tab, then try again.";
    const silent = (resp.status === 404 && method === "GET") || (quiet && quiet.includes(resp.status));
    if (!silent) toast(msg);
    throw fsError(code || CODES[resp.status] || "EIO");
  }

  // fileURL is the API address of the document at rel.
  function fileURL(rel) { return "/api/files/" + encodePath(rel); }

  // folderURL is the API address of the folder at rel.
  function folderURL(rel) { return "/api/folders/" + encodePath(rel); }

  // listing returns what the folder dir ("" for the top) holds:
  // { used, quota, dir, folders: [{name, path, modified}],
  // files: [{name, path, size, modified}] }. A missing folder throws
  // ENOENT.
  async function listing(dir) {
    return (await api("GET", "/api/files" + (dir ? "?dir=" + encodeURIComponent(dir) : ""))).json();
  }

  // move moves or renames a document or folder on the server.
  function move(from, to) {
    return api("POST", "/api/move", JSON.stringify({ from: from, to: to }));
  }

  // find returns the entry in list named name without regard to case.
  function find(list, name) {
    const lower = name.toLowerCase();
    return list.find(function (f) { return f.name.toLowerCase() === lower; });
  }

  // lookup returns the document or folder at p ({ file } or { folder })
  // from its folder's listing, or null when it or its folder is missing.
  async function lookup(p) {
    let l;
    try {
      l = await listing(p.dir);
    } catch (e) {
      if (e.code === "ENOENT") return null;
      throw e;
    }
    if (isDoc(p.name)) {
      const f = find(l.files, p.name);
      return f ? { file: f } : null;
    }
    const d = find(l.folders, p.name);
    return d ? { folder: d } : null;
  }

  // ---- the file system ----

  const S_IFDIR = 0o040000, S_IFREG = 0o100000;
  const O = { O_RDONLY: 0, O_WRONLY: 1, O_RDWR: 2, O_CREAT: 0o100, O_EXCL: 0o200, O_TRUNC: 0o1000, O_APPEND: 0o2000, O_DIRECTORY: 0o200000 };

  // A record is a file's bytes in memory: { rel, data, size, mtime,
  // server, dirty }. locals holds the page-only ones by path.
  const locals = new Map();
  // fds maps open file descriptors to { rec, pos, write, append, dir }.
  const fds = new Map();
  let nextFD = 100;

  // newRecord returns an empty record for the file at rel.
  function newRecord(rel, server) {
    return { rel: rel, data: new Uint8Array(0), size: 0, mtime: Date.now(), server: server, dirty: false };
  }

  // inode is a stable number for a path (any case), so os.SameFile works.
  function inode(rel) {
    let h = 2166136261;
    const s = rel.toLowerCase();
    for (let i = 0; i < s.length; i++) h = Math.imul(h ^ s.charCodeAt(i), 16777619) >>> 0;
    return h & 0x7fffffff;
  }

  // statOf builds the stat object Go reads (fs_js.go setStat).
  function statOf(isDir, rel, size, mtime) {
    return {
      dev: 1, ino: rel === "" ? 1 : inode(rel), mode: isDir ? S_IFDIR | 0o755 : S_IFREG | 0o644, nlink: 1,
      uid: 0, gid: 0, rdev: 0, size: size, blksize: 4096, blocks: Math.ceil(size / 512),
      atimeMs: mtime, mtimeMs: mtime, ctimeMs: mtime,
      isDirectory: function () { return isDir; },
    };
  }

  // ensure grows rec's buffer to hold n bytes.
  function ensure(rec, n) {
    if (n <= rec.data.length) return;
    const grown = new Uint8Array(Math.max(n, rec.data.length * 2, 4096));
    grown.set(rec.data.subarray(0, rec.size));
    rec.data = grown;
  }

  // upload stores rec's bytes as the server document at rel.
  async function upload(rec, rel) {
    await api("PUT", fileURL(rel), rec.data.slice(0, rec.size));
    rec.dirty = false;
  }

  // run calls fn's promise and hands the outcome to a Node-style callback.
  function run(callback, fn) {
    fn().then(function (v) { callback(null, v); }, function (e) {
      callback(e && e.code ? e : fsError("EIO"));
    });
  }

  // enosys fails a call on a path outside /server, as the stub does.
  function enosys(callback) { callback(fsError("ENOSYS")); }

  // Console output (fds 1 and 2), line by line, as wasm_exec.js's stub.
  const decoder = new TextDecoder("utf-8");
  let outputBuf = "";

  globalThis.fs = {
    constants: O,

    writeSync(fd, buf) {
      outputBuf += decoder.decode(buf);
      const nl = outputBuf.lastIndexOf("\n");
      if (nl !== -1) {
        console.log(outputBuf.substring(0, nl));
        outputBuf = outputBuf.substring(nl + 1);
      }
      return buf.length;
    },

    open(path, flags, mode, callback) {
      const p = parse(path);
      if (!p) return enosys(callback);
      run(callback, async function () {
        const acc = flags & 3, creat = flags & O.O_CREAT, excl = flags & O.O_EXCL, trunc = flags & O.O_TRUNC;
        const write = acc !== O.O_RDONLY;
        let rec = null, dir = false;
        if (p.root) {
          if (write) throw fsError("EISDIR");
          dir = true;
        } else if (!isDoc(p.name)) {
          rec = locals.get(p.rel);
          if (rec && creat && excl) throw fsError("EEXIST");
          if (!rec && (write || creat || hidden(p.name))) {
            // A page-only file: a temporary or lock file, or one the
            // server wouldn't keep.
            if (!creat) throw fsError("ENOENT");
            rec = newRecord(p.rel, false);
            locals.set(p.rel, rec);
          } else if (!rec) {
            const found = await lookup(p); // a folder, read
            if (!found || !found.folder) throw fsError("ENOENT");
            dir = true;
          }
        } else if (write && creat && trunc && !excl) {
          rec = newRecord(p.rel, true); // replaced whole: nothing to fetch
          rec.dirty = true;
        } else {
          if (creat && excl && await lookup(p)) throw fsError("EEXIST");
          try {
            const resp = await api("GET", fileURL(p.rel));
            rec = newRecord(p.rel, true);
            rec.data = new Uint8Array(await resp.arrayBuffer());
            rec.size = rec.data.length;
            const lm = Date.parse(resp.headers.get("Last-Modified") || "");
            if (!isNaN(lm)) rec.mtime = lm;
          } catch (e) {
            if (e.code !== "ENOENT" || !creat) throw e;
            rec = newRecord(p.rel, true);
            rec.dirty = true;
          }
        }
        if (rec && trunc && write) {
          rec.size = 0;
          rec.dirty = true;
        }
        const fd = nextFD++;
        fds.set(fd, { rec: rec, rel: p.rel, pos: 0, write: write, append: (flags & O.O_APPEND) !== 0, dir: dir });
        return fd;
      });
    },

    close(fd, callback) {
      const f = fds.get(fd);
      if (!f) return callback(fsError("EBADF"));
      fds.delete(fd);
      if (!f.rec || !f.rec.server || !f.rec.dirty) return callback(null);
      run(callback, function () { return upload(f.rec, f.rec.rel); });
    },

    read(fd, buffer, offset, length, position, callback) {
      const f = fds.get(fd);
      if (!f) return callback(fsError("EBADF"));
      if (f.dir) return callback(fsError("EISDIR"));
      const rec = f.rec;
      const pos = position === null || position === undefined ? f.pos : position;
      const n = Math.max(0, Math.min(length, rec.size - pos));
      buffer.set(rec.data.subarray(pos, pos + n), offset);
      if (position === null || position === undefined) f.pos += n;
      callback(null, n);
    },

    write(fd, buffer, offset, length, position, callback) {
      if (fd === 1 || fd === 2) {
        callback(null, this.writeSync(fd, buffer.subarray(offset, offset + length)));
        return;
      }
      const f = fds.get(fd);
      if (!f || !f.write) return callback(fsError("EBADF"));
      const rec = f.rec;
      let pos = position === null || position === undefined ? f.pos : position;
      if (f.append) pos = rec.size;
      ensure(rec, pos + length);
      rec.data.set(buffer.subarray(offset, offset + length), pos);
      rec.size = Math.max(rec.size, pos + length);
      rec.mtime = Date.now();
      rec.dirty = true;
      if (position === null || position === undefined || f.append) f.pos = pos + length;
      callback(null, length);
    },

    fstat(fd, callback) {
      const f = fds.get(fd);
      if (!f) return callback(fsError("EBADF"));
      if (f.dir) return callback(null, statOf(true, f.rel, 0, Date.now()));
      callback(null, statOf(false, f.rec.rel, f.rec.size, f.rec.mtime));
    },

    stat(path, callback) {
      const p = parse(path);
      if (!p) return enosys(callback);
      run(callback, async function () {
        if (p.root) return statOf(true, "", 0, Date.now());
        const rec = locals.get(p.rel);
        if (rec) return statOf(false, rec.rel, rec.size, rec.mtime);
        if (hidden(p.name)) throw fsError("ENOENT");
        const found = await lookup(p);
        if (!found) throw fsError("ENOENT");
        if (found.folder) return statOf(true, found.folder.path, 0, found.folder.modified);
        return statOf(false, found.file.path, found.file.size, found.file.modified);
      });
    },

    lstat(path, callback) { this.stat(path, callback); },

    readdir(path, callback) {
      const p = parse(path);
      if (!p) return enosys(callback);
      if (!p.root && isDoc(p.name)) return callback(fsError("ENOTDIR"));
      run(callback, async function () {
        const l = await listing(p.rel);
        return l.folders.map(function (f) { return f.name; }).concat(l.files.map(function (f) { return f.name; }));
      });
    },

    rename(from, to, callback) {
      const a = parse(from), b = parse(to);
      if (!a || !b) return enosys(callback);
      if (a.root || b.root) return callback(fsError("EINVAL"));
      run(callback, async function () {
        const rec = locals.get(a.rel);
        if (rec) {
          if (!isDoc(b.name)) {
            locals.delete(a.rel);
            rec.rel = b.rel;
            locals.set(b.rel, rec);
            return;
          }
          // A safe save's temporary file renamed over the document.
          await upload(rec, b.rel);
          locals.delete(a.rel);
          return;
        }
        if (hidden(a.name) || hidden(b.name)) throw fsError(hidden(a.name) ? "ENOENT" : "ENOSYS");
        if (isDoc(a.name) !== isDoc(b.name)) throw fsError("EINVAL"); // a document and a folder
        await move(a.rel, b.rel);
      });
    },

    unlink(path, callback) {
      const p = parse(path);
      if (!p) return enosys(callback);
      if (p.root) return callback(fsError("EISDIR"));
      if (locals.delete(p.rel)) return callback(null);
      if (!isDoc(p.name)) return callback(fsError(hidden(p.name) ? "ENOENT" : "EISDIR")); // os.Remove then tries rmdir
      run(callback, function () { return api("DELETE", fileURL(p.rel)); });
    },

    mkdir(path, perm, callback) {
      const p = parse(path);
      if (!p) return enosys(callback);
      if (p.root) return callback(fsError("EEXIST"));
      if (hidden(p.name) || isDoc(p.name)) return callback(fsError("EINVAL"));
      run(callback, function () { return api("PUT", folderURL(p.rel), undefined, [409]); });
    },

    rmdir(path, callback) {
      const p = parse(path);
      if (!p) return enosys(callback);
      if (p.root) return callback(fsError("EBUSY"));
      if (hidden(p.name) || isDoc(p.name)) return callback(fsError("ENOTDIR"));
      run(callback, function () { return api("DELETE", folderURL(p.rel), undefined, [409]); });
    },

    ftruncate(fd, length, callback) {
      const f = fds.get(fd);
      if (!f || !f.write) return callback(fsError("EBADF"));
      ensure(f.rec, length);
      if (length > f.rec.size) f.rec.data.fill(0, f.rec.size, length);
      f.rec.size = length;
      f.rec.dirty = true;
      callback(null);
    },

    fsync(fd, callback) { callback(null); }, // written on close or rename

    // Permissions, owners and times mean nothing on the server: accepted
    // for /server paths and ignored.
    chmod(path, mode, callback) { parse(path) ? callback(null) : enosys(callback); },
    fchmod(fd, mode, callback) { callback(null); },
    chown(path, uid, gid, callback) { parse(path) ? callback(null) : enosys(callback); },
    fchown(fd, uid, gid, callback) { callback(null); },
    lchown(path, uid, gid, callback) { parse(path) ? callback(null) : enosys(callback); },
    utimes(path, atime, mtime, callback) { parse(path) ? callback(null) : enosys(callback); },

    truncate(path, length, callback) { enosys(callback); },
    readlink(path, callback) { parse(path) ? callback(fsError("EINVAL")) : enosys(callback); },
    link(path, link, callback) { enosys(callback); },
    symlink(path, link, callback) { enosys(callback); },
  };

  // ---- messages ----

  let toastTimer = 0;

  // toast shows msg at the bottom of the page for a few seconds.
  function toast(msg) {
    if (!hasDOM) return;
    let el = document.getElementById("fs-toast");
    if (!el) {
      el = document.createElement("div");
      el.id = "fs-toast";
      el.className = "err toast";
      el.setAttribute("role", "alert");
      document.body.appendChild(el);
    }
    el.textContent = msg;
    el.hidden = false;
    clearTimeout(toastTimer);
    toastTimer = setTimeout(function () { el.hidden = true; }, 8000);
  }

  // openTab opens url (an app page) in a new tab, as the app does for a
  // second file; label names what opens there.
  function openTab(url, label) {
    if (!window.open(url, "_blank")) askNewTab(url, label);
  }

  // askNewTab asks to open url in a new tab, for a browser that blocked
  // it: a page may open a tab only just after a click (picking a file
  // takes longer), and the dialog's button is that click.
  function askNewTab(url, label) {
    modal("Open in a new tab", function (box, close) {
      box.appendChild(el("p", "", label.charAt(0).toUpperCase() + label.slice(1) + " opens in a new tab."));
      const row = el("div", "fd-buttons");
      row.append(
        button("Cancel", "ghost", function () { close("cancel"); }),
        button("Open in a new tab", "", function () {
          window.open(url, "_blank"); // within this click, so allowed
          close("open");
        }));
      box.appendChild(row);
    });
  }

  // Files from the user's computer waiting for the tab opened for each
  // (openDataTab), by the id in that tab's address.
  const handoffs = new Map();

  // openDataTab opens app ("word" or "excel") in a new tab with data, a
  // file from the user's computer: the picker gave no path, so the new
  // tab asks this one for the contents (app.js, ?local=ID) and gets them
  // by postMessage, which only this site's own pages can exchange.
  function openDataTab(app, data) {
    const id = Date.now().toString(36) + Math.random().toString(36).slice(2);
    const url = "/" + app + "?local=" + id;
    handoffs.set(id, data);
    setTimeout(function () { handoffs.delete(id); }, HANDOFF_MS);
    if (!window.open(url, "_blank")) askNewTab(url, "the file from your computer");
  }

  // A tab opened by openDataTab asking for its file.
  if (hasDOM) {
    window.addEventListener("message", function (e) {
      const m = e.data;
      if (e.origin !== location.origin || !m || m.type !== "officeweb-ready" || !e.source) return;
      const data = handoffs.get(m.id);
      if (!data) return;
      handoffs.delete(m.id);
      e.source.postMessage({ type: "officeweb-file", id: m.id, data: data }, location.origin);
    });
  }

  // ---- dialogs ----

  // el makes an element with a class and text.
  function el(tag, cls, text) {
    const e = document.createElement(tag);
    if (cls) e.className = cls;
    if (text !== undefined) e.textContent = text;
    return e;
  }

  // button makes a button; kind is "", "ghost" or "danger".
  function button(text, kind, onClick) {
    const b = el("button", kind, text);
    b.type = "button";
    b.addEventListener("click", onClick);
    return b;
  }

  // formatSize shows a byte count like the server pages (KiB, MiB, …).
  function formatSize(n) {
    if (n < 1024) return n + " B";
    const units = ["KiB", "MiB", "GiB", "TiB"];
    let u = -1;
    do { n /= 1024; u++; } while (n >= 1024 && u < units.length - 1);
    return (Math.round(n * 10) / 10) + " " + units[u];
  }

  // modal shows a dialog box built by build(box, close) over the page and
  // resolves with what close was called with. While it is open the app
  // leaves the keyboard alone (common/gui/keyboard_js.go).
  function modal(title, build) {
    return new Promise(function (resolve) {
      const back = el("div", "fd-back");
      const box = el("div", "fd");
      box.setAttribute("role", "dialog");
      box.setAttribute("aria-modal", "true");
      box.appendChild(el("h2", "", title));
      back.appendChild(box);
      const before = document.activeElement;
      let done = false;
      function close(result) {
        if (done) return;
        done = true;
        back.remove();
        globalThis.officewebModalOpen = false;
        if (before && before.focus) before.focus();
        resolve(result);
      }
      back.addEventListener("keydown", function (e) {
        e.stopPropagation(); // keys belong to the dialog, not the app
        if (e.key === "Escape") close(box.dataset.cancel || "cancel");
      });
      globalThis.officewebModalOpen = true;
      build(box, close);
      document.body.appendChild(back);
      const first = Array.from(box.querySelectorAll("input, [aria-selected], button")).find(function (e) {
        return e.offsetParent !== null; // shown, not in a hidden part
      });
      if (first) first.focus();
    });
  }

  // lastDir is the folder the dialogs were last in, where the next one
  // starts.
  let lastDir = "";

  // browser adds a folder browser to box: the path from the top (each
  // step goes back there), the folder's subfolders and documents (with
  // extensions in exts, or all when exts is empty), a New folder button
  // and the quota. A click on a folder goes into it; pick(path, name,
  // open) is called when a document is clicked (open: double-clicked or
  // Enter). It starts in start ("" for the top; the top when start is
  // gone) and returns { dir() }, the folder shown.
  function browser(box, exts, start, pick) {
    let dir = start;
    const crumbs = el("nav", "crumbs");
    crumbs.setAttribute("aria-label", "Folder");
    const list = el("ul", "fd-list");
    list.setAttribute("role", "listbox");
    const quota = el("p", "hint");
    const newRow = el("div", "fd-newfolder");
    const newName = el("input");
    newName.placeholder = "New folder's name";
    newName.maxLength = MAX_NAME + 2;
    newName.setAttribute("autocomplete", "off");
    newName.spellcheck = false;
    const newProblem = el("p", "err");
    newProblem.hidden = true;
    const make = button("Make folder", "ghost", makeFolder);
    const cancelNew = button("Cancel", "ghost", function () { showNew(false); });
    newRow.append(newName, make, cancelNew);
    const newButton = button("New folder", "ghost", function () { showNew(true); });
    box.append(crumbs, list, newRow, newProblem, quota);

    // showNew shows or hides the new folder's name box.
    function showNew(on) {
      newRow.hidden = !on;
      newProblem.hidden = true;
      newButton.hidden = on;
      if (on) {
        newName.value = "";
        newName.focus();
      }
    }

    // depth is how many folders deep path is.
    function depth(path) { return path === "" ? 0 : path.split("/").length; }

    // makeFolder makes the typed folder in this one and goes into it.
    async function makeFolder() {
      const name = newName.value.trim();
      const why = folderProblem(name);
      if (why) {
        newProblem.textContent = why;
        newProblem.hidden = false;
        newName.focus();
        return;
      }
      try {
        await api("PUT", folderURL(join(dir, name)), undefined, [409]);
      } catch (e) {
        newProblem.textContent = e.code === "EEXIST" ? name + " already exists here." : "Couldn't make the folder.";
        newProblem.hidden = false;
        return;
      }
      showNew(false);
      show(join(dir, name));
    }
    newName.addEventListener("keydown", function (e) {
      if (e.key === "Enter") { e.preventDefault(); makeFolder(); }
      if (e.key === "Escape") { e.stopPropagation(); showNew(false); }
    });

    // option adds a list entry labeled text that calls choose(open).
    function option(text, cls, size, choose) {
      const li = el("li", cls);
      li.setAttribute("role", "option");
      li.tabIndex = 0;
      li.append(el("span", "", text), el("span", "size", size));
      const select = function () {
        list.querySelectorAll("[aria-selected]").forEach(function (o) { o.removeAttribute("aria-selected"); });
        li.setAttribute("aria-selected", "true");
      };
      li.addEventListener("click", function () { select(); choose(false); });
      li.addEventListener("dblclick", function () { select(); choose(true); });
      li.addEventListener("keydown", function (e) {
        if (e.key === "Enter") { select(); choose(true); }
      });
      list.appendChild(li);
    }

    // drawCrumbs shows the path to the folder shown.
    function drawCrumbs(shown) {
      crumbs.replaceChildren();
      const steps = [{ name: "Cloud files", path: "" }];
      let at = "";
      if (shown !== "") shown.split("/").forEach(function (n) { at = join(at, n); steps.push({ name: n, path: at }); });
      steps.forEach(function (st, i) {
        if (i > 0) crumbs.appendChild(el("span", "", "›"));
        if (i === steps.length - 1) {
          crumbs.appendChild(el("strong", "", st.name));
        } else {
          crumbs.appendChild(button(st.name, "ghost", function () { show(st.path); }));
        }
      });
    }

    // show lists the folder at path (the top when it's gone).
    async function show(path) {
      list.replaceChildren(el("li", "empty", "Loading…"));
      let l;
      try {
        l = await listing(path);
      } catch (e) {
        if (e.code === "ENOENT" && path !== "") return show("");
        list.replaceChildren(el("li", "empty", "Couldn't list your cloud files."));
        return;
      }
      dir = l.dir;
      lastDir = dir;
      drawCrumbs(dir);
      list.replaceChildren();
      if (dir !== "") {
        option("Up to " + parentName(dir), "up", "", function () { show(parentOf(dir)); });
      }
      l.folders.forEach(function (f) {
        option(f.name, "folder", "", function () { show(f.path); });
      });
      const shown = l.files.filter(function (f) { return exts.length === 0 || exts.includes(extOf(f.name)); });
      shown.forEach(function (f) {
        option(f.name, "", formatSize(f.size), function (open) { pick(f.path, f.name, open); });
      });
      if (l.folders.length === 0 && shown.length === 0) {
        list.appendChild(el("li", "empty", dir === "" ? "No cloud files yet." : "This folder is empty."));
      }
      newButton.disabled = depth(dir) >= MAX_DEPTH;
      newButton.title = newButton.disabled ? "Folders may be at most " + MAX_DEPTH + " deep." : "";
      quota.textContent = "Using " + formatSize(l.used) + " of " + formatSize(l.quota) + " of cloud storage.";
    }

    showNew(false);
    show(start);
    return { dir: function () { return dir; }, newButton: newButton };
  }

  // parentOf is the folder holding path ("" for the top).
  function parentOf(path) {
    const i = path.lastIndexOf("/");
    return i < 0 ? "" : path.slice(0, i);
  }

  // parentName is the name of the folder holding path, or "Cloud files"
  // for the top.
  function parentName(path) {
    const up = parentOf(path);
    return up === "" ? "Cloud files" : up.slice(up.lastIndexOf("/") + 1);
  }

  // anyStorable reports whether exts is empty (all files) or names a type
  // the server keeps.
  function anyStorable(exts) {
    return exts.length === 0 || exts.some(function (e) { return EXTS.includes(e.toLowerCase()); });
  }

  // openDialog asks which cloud file to open, starting in the folder the
  // dialogs were last in. It resolves with its path, "cancel", or "local"
  // (open a file from this computer instead).
  function openDialog(_, exts) {
    exts = (exts || []).map(function (e) { return e.toLowerCase(); });
    if (!anyStorable(exts)) return Promise.resolve("local"); // pictures and the like
    return modal("Open from cloud files", function (box, close) {
      let chosen = "";
      const ok = button("Open", "", function () { if (chosen) close(MOUNT + "/" + chosen); });
      ok.disabled = true;
      const b = browser(box, exts, lastDir, function (path, name, open) {
        chosen = path;
        ok.disabled = false;
        if (open) close(MOUNT + "/" + path);
      });
      const row = el("div", "fd-buttons");
      row.append(
        button("From this computer…", "ghost local", function () { close("local"); }),
        b.newButton,
        button("Cancel", "ghost", function () { close("cancel"); }),
        ok);
      box.appendChild(row);
    });
  }

  // saveDialog asks for the cloud file to save as, suggesting name: a
  // /server path starts in its folder, anything else in the folder the
  // dialogs were last in. A name typed without an extension gets exts'
  // first. It asks before replacing a file and resolves with the path,
  // "cancel", or "local" (download to this computer instead).
  function saveDialog(name, exts) {
    exts = (exts || []).map(function (e) { return e.toLowerCase(); });
    if (!anyStorable(exts)) return Promise.resolve("local");
    let start = lastDir;
    name = name || "";
    const p = parse(name);
    if (p && !p.root) {
      start = p.dir;
      name = p.name;
    } else if (name.includes("/")) {
      name = name.slice(name.lastIndexOf("/") + 1);
    }
    return modal("Save to cloud files", function (box, close) {
      const label = el("label", "fd-name", "File name");
      const input = el("input");
      input.value = name;
      input.maxLength = MAX_NAME + 2; // room for spaces that get trimmed
      input.setAttribute("autocomplete", "off");
      input.spellcheck = false;
      label.appendChild(input);
      const problem = el("p", "err");
      problem.hidden = true;
      let replacing = "";
      const save = button("Save", "", submit);
      let b = null;

      // fixed is the typed name, trimmed, with the default extension
      // added when it has none.
      function fixed() {
        let n = input.value.trim();
        if (n !== "" && extOf(n) === "" && exts.length > 0) n += exts[0];
        return n;
      }

      // submit checks the name, asks before replacing, and closes.
      async function submit() {
        const n = fixed();
        const why = nameProblem(n);
        if (why) {
          problem.textContent = why;
          problem.hidden = false;
          input.focus();
          return;
        }
        const dir = b.dir();
        if (replacing && replacing.toLowerCase() === join(dir, n).toLowerCase()) {
          close(MOUNT + "/" + replacing);
          return;
        }
        let existing;
        try {
          existing = find((await listing(dir)).files, n);
        } catch (e) {
          problem.textContent = "Couldn't reach the server.";
          problem.hidden = false;
          return;
        }
        if (existing) {
          replacing = existing.path;
          problem.textContent = existing.name + " already exists here. Click Replace to save over it.";
          problem.hidden = false;
          save.textContent = "Replace";
          return;
        }
        close(MOUNT + "/" + join(dir, n));
      }

      // reset forgets a Replace asked for, after the name or folder changed.
      function reset() {
        problem.hidden = true;
        replacing = "";
        save.textContent = "Save";
      }

      input.addEventListener("input", reset);
      input.addEventListener("keydown", function (e) { if (e.key === "Enter") submit(); });
      b = browser(box, exts, start, function (path, picked, open) {
        input.value = picked;
        reset();
        if (open) submit();
      });
      box.querySelector(".fd-list").addEventListener("click", function (e) {
        if (e.target.closest(".folder, .up")) reset();
      });
      box.appendChild(label);
      box.appendChild(problem);
      const hint = el("p", "hint", "Up to " + MAX_NAME + " characters of printable ASCII; not / \\ or *. Spaces at the ends are trimmed.");
      box.appendChild(hint);
      const row = el("div", "fd-buttons");
      row.append(
        button("Download to this computer", "ghost local", function () { close("local"); }),
        b.newButton,
        button("Cancel", "ghost", function () { close("cancel"); }),
        save);
      box.appendChild(row);
      setTimeout(function () {
        input.focus();
        const dot = input.value.lastIndexOf(".");
        input.setSelectionRange(0, dot > 0 ? dot : input.value.length);
      }, 0);
    });
  }

  // ---- Yes/No confirmation for the server pages' forms ----

  // confirmBox asks question with Yes and No; it resolves true for Yes.
  function confirmBox(question) {
    return modal("Are you sure?", function (box, close) {
      box.dataset.cancel = "no";
      box.appendChild(el("p", "", question));
      const row = el("div", "fd-buttons");
      row.append(
        button("No", "ghost", function () { close("no"); }),
        button("Yes", "danger", function () { close("yes"); }));
      box.appendChild(row);
      setTimeout(function () { row.firstChild.focus(); }, 0); // No: the safe default
    }).then(function (r) { return r === "yes"; });
  }

  // A form with data-confirm="question" asks first; Yes submits it with
  // confirm=1, which the server requires.
  if (hasDOM) document.addEventListener("submit", function (e) {
    const form = e.target;
    if (!(form instanceof HTMLFormElement) || !form.dataset.confirm || form.dataset.confirmed) return;
    e.preventDefault();
    confirmBox(form.dataset.confirm).then(function (yes) {
      if (!yes) return;
      const field = document.createElement("input");
      field.type = "hidden";
      field.name = "confirm";
      field.value = "1";
      form.appendChild(field);
      form.dataset.confirmed = "1";
      form.submit();
    });
  });

  globalThis.officeweb = { openDialog: openDialog, saveDialog: saveDialog, confirm: confirmBox, openTab: openTab, openDataTab: openDataTab };
})();
