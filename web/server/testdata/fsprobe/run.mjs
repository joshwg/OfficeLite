// run.mjs runs fsprobe.wasm under Node with serverfs.js as its file
// system, against the test server at BASE, logged in with COOKIE:
//   node run.mjs SERVERFS_JS WASM_EXEC_JS PROBE_WASM
// fetch is wrapped to add the server's address and the session cookie,
// which a browser would supply. Exits with the probe's exit code.
import { readFileSync } from "node:fs";
import vm from "node:vm";

const [serverfs, wasmExec, probe] = process.argv.slice(2);
const base = process.env.BASE;
const cookie = process.env.COOKIE;
const realFetch = globalThis.fetch;
globalThis.fetch = (url, init = {}) =>
  realFetch(base + url, { ...init, headers: { ...(init.headers || {}), Cookie: cookie } });

// The file-name rules a browser gets from serverfs.js's script tag.
globalThis.officewebRules = { maxName: process.env.MAX_NAME, exts: process.env.EXTS };

// Node's own process stays (wasm_exec.js uses it when present); serverfs
// replaces fs before wasm_exec.js would install its stub.
vm.runInThisContext(readFileSync(serverfs, "utf8"), { filename: serverfs });
vm.runInThisContext(readFileSync(wasmExec, "utf8"), { filename: wasmExec });

const go = new globalThis.Go();
go.argv = ["fsprobe"];
let code = 0;
go.exit = (c) => { code = c; };
const { instance } = await WebAssembly.instantiate(readFileSync(probe), go.importObject);
await go.run(instance);
process.exit(code);
