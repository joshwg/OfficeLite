// serverfs.js gives the WebAssembly client the user's server documents
// as files. Go's os package in the browser calls a Node-style globalThis.fs
// (wasm_exec.js puts a stub there that fails everything); this script,
// loaded first, puts a file system there instead in which /server is a
// folder of the user's documents, read and written through the server's
// /api/files. So WordLite and ExcelLite open, save and AutoSave
// /server/Budget.xlsx with the same code that handles files on a desktop.
//
//   * A document is fetched whole when opened and kept in memory; what is
//     written goes to the server when the file is closed (or, for the
//     hidden temporary file a safe save writes first, when it is renamed
//     over the document).
//   * Names the server never stores (starting with "." or "~$", or with an
//     extension it doesn't keep, such as the ".tmp" of temporary files and
//     the lock files) never reach it: they live in this page's memory only.
//   * Every path outside /server fails with ENOSYS, as with the stub.
//
// It also offers the apps' Open and Save As dialogs of server files
// (window.officeweb.openDialog / saveDialog, used by
// common/gui/filedialog_js.go), and Yes/No confirmation for the server
// pages' forms marked data-confirm. The file-name rules come from the
// server through this script tag's data-max-name and data-exts (the
// server checks them again).
(function () {
  "use strict";
  const MOUNT = "/server";
  // No document under Node, where serverfs_test.go runs this file.
  const hasDOM = typeof document !== "undefined";
  const script = hasDOM ? document.currentScript : null;
  // The rules from the script tag, or (under Node) from
  // globalThis.officewebRules = { maxName, exts }.
  const rules = script ? script.dataset : (globalThis.officewebRules || {});
  const MAX_NAME = parseInt(rules.maxName || "64", 10);
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

  // nameProblem returns why name (already trimmed) can't be a server
  // document's name, or "" when it can: the rules of web/names.
  function nameProblem(name) {
    if (name === "") return "Type a file name.";
    if (name.length > MAX_NAME) return "File names may have at most " + MAX_NAME + " characters.";
    for (let i = 0; i < name.length; i++) {
      const c = name.charCodeAt(i);
      if (c < 0x20 || c > 0x7e) return "File names may contain only printable ASCII characters (no control, invisible or accented characters).";
    }
    if (/[\/\\*]/.test(name)) return "File names may not contain '/', '\\' or '*'.";
    if (name.startsWith(".") || name.startsWith("~$")) return "File names may not start with '.' or '~$'.";
    if (!storable(name)) return "File names must end in one of " + EXTS.join(" ") + ".";
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

  // localOnly reports whether name stays in this page's memory: a
  // temporary or lock file, or anything else the server wouldn't store.
  function localOnly(name) {
    return name.startsWith(".") || name.startsWith("~$") || !storable(name);
  }

  // parse splits a path into { dir: true } for /server itself,
  // { name } for a file in it, or null for anything else.
  function parse(path) {
    if (typeof path !== "string") return null;
    if (path === MOUNT || path === MOUNT + "/") return { dir: true };
    if (!path.startsWith(MOUNT + "/")) return null;
    const name = path.slice(MOUNT.length + 1);
    if (name === "" || name.includes("/")) return null;
    return { name: name };
  }

  // ---- the server ----

  // api calls /api/files[/name][suffix] and returns the response, or
  // throws an fsError (after telling the user what the server said).
  async function api(method, name, body, suffix) {
    let url = "/api/files";
    if (name !== undefined) url += "/" + encodeURIComponent(name);
    if (suffix) url += suffix;
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
    if (resp.status !== 404 || method !== "GET") {
      let msg = resp.statusText;
      try { msg = (await resp.json()).error || msg; } catch (e) { /* not JSON */ }
      if (resp.status === 401) msg = "You've been logged out. Log in again in another tab, then try again.";
      toast(msg);
    }
    throw fsError(CODES[resp.status] || "EIO");
  }

  // listing returns { used, quota, files: [{name, size, modified}] }.
  async function listing() {
    return (await api("GET")).json();
  }

  // find returns the listed file named name without regard to case.
  function find(files, name) {
    const lower = name.toLowerCase();
    return files.find(function (f) { return f.name.toLowerCase() === lower; });
  }

  // ---- the file system ----

  const S_IFDIR = 0o040000, S_IFREG = 0o100000;
  const O = { O_RDONLY: 0, O_WRONLY: 1, O_RDWR: 2, O_CREAT: 0o100, O_EXCL: 0o200, O_TRUNC: 0o1000, O_APPEND: 0o2000, O_DIRECTORY: 0o200000 };

  // A record is a file's bytes in memory: { name, data, size, mtime,
  // server, dirty }. locals holds the local-only ones by name.
  const locals = new Map();
  // fds maps open file descriptors to { rec, pos, write, append, dir }.
  const fds = new Map();
  let nextFD = 100;

  // newRecord returns an empty record for name.
  function newRecord(name, server) {
    return { name: name, data: new Uint8Array(0), size: 0, mtime: Date.now(), server: server, dirty: false };
  }

  // inode is a stable number for a name (any case), so os.SameFile works.
  function inode(name) {
    let h = 2166136261;
    const s = name.toLowerCase();
    for (let i = 0; i < s.length; i++) h = Math.imul(h ^ s.charCodeAt(i), 16777619) >>> 0;
    return h & 0x7fffffff;
  }

  // statOf builds the stat object Go reads (fs_js.go setStat).
  function statOf(isDir, name, size, mtime) {
    return {
      dev: 1, ino: isDir ? 1 : inode(name), mode: isDir ? S_IFDIR | 0o755 : S_IFREG | 0o644, nlink: 1,
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

  // upload stores rec's bytes as the server document name.
  async function upload(rec, name) {
    await api("PUT", name, rec.data.slice(0, rec.size));
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
        let rec;
        if (p.dir) {
          if (write) throw fsError("EISDIR");
          rec = null;
        } else if (localOnly(p.name)) {
          rec = locals.get(p.name);
          if (rec && creat && excl) throw fsError("EEXIST");
          if (!rec) {
            if (!creat) throw fsError("ENOENT");
            rec = newRecord(p.name, false);
            locals.set(p.name, rec);
          }
        } else if (write && creat && trunc && !excl) {
          rec = newRecord(p.name, true); // replaced whole: nothing to fetch
          rec.dirty = true;
        } else {
          if (creat && excl && find((await listing()).files, p.name)) throw fsError("EEXIST");
          try {
            const resp = await api("GET", p.name);
            rec = newRecord(p.name, true);
            rec.data = new Uint8Array(await resp.arrayBuffer());
            rec.size = rec.data.length;
            const lm = Date.parse(resp.headers.get("Last-Modified") || "");
            if (!isNaN(lm)) rec.mtime = lm;
          } catch (e) {
            if (e.code !== "ENOENT" || !creat) throw e;
            rec = newRecord(p.name, true);
            rec.dirty = true;
          }
        }
        if (rec && trunc && write) {
          rec.size = 0;
          rec.dirty = true;
        }
        const fd = nextFD++;
        fds.set(fd, { rec: rec, pos: 0, write: write, append: (flags & O.O_APPEND) !== 0, dir: p.dir === true });
        return fd;
      });
    },

    close(fd, callback) {
      const f = fds.get(fd);
      if (!f) return callback(fsError("EBADF"));
      fds.delete(fd);
      if (!f.rec || !f.rec.server || !f.rec.dirty) return callback(null);
      run(callback, function () { return upload(f.rec, f.rec.name); });
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
      if (f.dir) return callback(null, statOf(true, "", 0, Date.now()));
      callback(null, statOf(false, f.rec.name, f.rec.size, f.rec.mtime));
    },

    stat(path, callback) {
      const p = parse(path);
      if (!p) return enosys(callback);
      run(callback, async function () {
        if (p.dir) return statOf(true, "", 0, Date.now());
        if (localOnly(p.name)) {
          const rec = locals.get(p.name);
          if (!rec) throw fsError("ENOENT");
          return statOf(false, rec.name, rec.size, rec.mtime);
        }
        const f = find((await listing()).files, p.name);
        if (!f) throw fsError("ENOENT");
        return statOf(false, f.name, f.size, f.modified);
      });
    },

    lstat(path, callback) { this.stat(path, callback); },

    readdir(path, callback) {
      const p = parse(path);
      if (!p) return enosys(callback);
      if (!p.dir) return callback(fsError("ENOTDIR"));
      run(callback, async function () {
        return (await listing()).files.map(function (f) { return f.name; });
      });
    },

    rename(from, to, callback) {
      const a = parse(from), b = parse(to);
      if (!a || !b) return enosys(callback);
      if (a.dir || b.dir) return callback(fsError("EINVAL"));
      run(callback, async function () {
        if (localOnly(a.name)) {
          const rec = locals.get(a.name);
          if (!rec) throw fsError("ENOENT");
          if (localOnly(b.name)) {
            locals.delete(a.name);
            rec.name = b.name;
            locals.set(b.name, rec);
            return;
          }
          // A safe save's temporary file renamed over the document.
          await upload(rec, b.name);
          locals.delete(a.name);
          return;
        }
        if (localOnly(b.name)) throw fsError("ENOSYS");
        await api("POST", a.name, JSON.stringify({ to: b.name }), "/rename");
      });
    },

    unlink(path, callback) {
      const p = parse(path);
      if (!p) return enosys(callback);
      if (p.dir) return callback(fsError("EISDIR"));
      if (localOnly(p.name)) {
        if (!locals.delete(p.name)) return callback(fsError("ENOENT"));
        return callback(null);
      }
      run(callback, function () { return api("DELETE", p.name); });
    },

    mkdir(path, perm, callback) {
      const p = parse(path);
      if (!p) return enosys(callback);
      callback(fsError(p.dir ? "EEXIST" : "ENOSYS"));
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
    rmdir(path, callback) { enosys(callback); },
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
      const first = box.querySelector("input, [aria-selected], button");
      if (first) first.focus();
    });
  }

  // fileList adds the user's server files (with extensions in exts, or all
  // when exts is empty) to box as a list; pick(name, open) is called when
  // one is clicked (open: double-clicked or Enter).
  function fileList(box, exts, pick) {
    const list = el("ul", "fd-list");
    list.setAttribute("role", "listbox");
    list.appendChild(el("li", "empty", "Loading…"));
    box.appendChild(list);
    const quota = el("p", "hint");
    box.appendChild(quota);
    listing().then(function (l) {
      list.replaceChildren();
      const shown = l.files.filter(function (f) { return exts.length === 0 || exts.includes(extOf(f.name)); });
      if (shown.length === 0) list.appendChild(el("li", "empty", "No files on the server yet."));
      shown.forEach(function (f) {
        const li = el("li");
        li.setAttribute("role", "option");
        li.tabIndex = 0;
        li.append(el("span", "", f.name), el("span", "size", formatSize(f.size)));
        const select = function () {
          list.querySelectorAll("[aria-selected]").forEach(function (o) { o.removeAttribute("aria-selected"); });
          li.setAttribute("aria-selected", "true");
        };
        li.addEventListener("click", function () { select(); pick(f.name, false); });
        li.addEventListener("dblclick", function () { select(); pick(f.name, true); });
        li.addEventListener("keydown", function (e) {
          if (e.key === "Enter") { select(); pick(f.name, true); }
        });
        list.appendChild(li);
      });
      quota.textContent = "Using " + formatSize(l.used) + " of " + formatSize(l.quota) + " on the server.";
    }, function () {
      list.replaceChildren(el("li", "empty", "Couldn't list your server files."));
    });
  }

  // anyStorable reports whether exts is empty (all files) or names a type
  // the server keeps.
  function anyStorable(exts) {
    return exts.length === 0 || exts.some(function (e) { return EXTS.includes(e.toLowerCase()); });
  }

  // openDialog asks which server file to open. It resolves with its path,
  // "cancel", or "local" (open a file from this computer instead).
  function openDialog(_, exts) {
    exts = (exts || []).map(function (e) { return e.toLowerCase(); });
    if (!anyStorable(exts)) return Promise.resolve("local"); // pictures and the like
    return modal("Open", function (box, close) {
      let chosen = "";
      const ok = button("Open", "", function () { if (chosen) close(MOUNT + "/" + chosen); });
      ok.disabled = true;
      fileList(box, exts, function (name, open) {
        chosen = name;
        ok.disabled = false;
        if (open) close(MOUNT + "/" + name);
      });
      const row = el("div", "fd-buttons");
      row.append(
        button("From this computer…", "ghost local", function () { close("local"); }),
        button("Cancel", "ghost", function () { close("cancel"); }),
        ok);
      box.appendChild(row);
    });
  }

  // saveDialog asks for the server file to save as, suggesting name; a
  // name typed without an extension gets exts' first. It asks before
  // replacing a file and resolves with the path, "cancel", or "local"
  // (download to this computer instead).
  function saveDialog(name, exts) {
    exts = (exts || []).map(function (e) { return e.toLowerCase(); });
    if (!anyStorable(exts)) return Promise.resolve("local");
    return modal("Save As", function (box, close) {
      const label = el("label", "fd-name", "File name on the server");
      const input = el("input");
      input.value = name || "";
      input.maxLength = MAX_NAME + 2; // room for spaces that get trimmed
      input.setAttribute("autocomplete", "off");
      input.spellcheck = false;
      label.appendChild(input);
      const problem = el("p", "err");
      problem.hidden = true;
      let replacing = "";
      const save = button("Save", "", submit);

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
        if (replacing && replacing.toLowerCase() === n.toLowerCase()) {
          close(MOUNT + "/" + replacing);
          return;
        }
        let existing;
        try {
          existing = find((await listing()).files, n);
        } catch (e) {
          problem.textContent = "Couldn't reach the server.";
          problem.hidden = false;
          return;
        }
        if (existing) {
          replacing = existing.name;
          problem.textContent = existing.name + " already exists. Click Replace to save over it.";
          problem.hidden = false;
          save.textContent = "Replace";
          return;
        }
        close(MOUNT + "/" + n);
      }

      input.addEventListener("input", function () {
        problem.hidden = true;
        replacing = "";
        save.textContent = "Save";
      });
      input.addEventListener("keydown", function (e) { if (e.key === "Enter") submit(); });
      fileList(box, exts, function (picked, open) {
        input.value = picked;
        input.dispatchEvent(new Event("input"));
        if (open) submit();
      });
      box.appendChild(label);
      box.appendChild(problem);
      const hint = el("p", "hint", "Up to " + MAX_NAME + " characters of printable ASCII; not / \\ or *. Spaces at the ends are trimmed.");
      box.appendChild(hint);
      const row = el("div", "fd-buttons");
      row.append(
        button("Download to this computer", "ghost local", function () { close("local"); }),
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

  globalThis.officeweb = { openDialog: openDialog, saveDialog: saveDialog, confirm: confirmBox };
})();
