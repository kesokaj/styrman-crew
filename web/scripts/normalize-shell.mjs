// TanStack's prerenderer stamps the router payload in dist/client/_shell.html
// with a wall-clock millisecond timestamp (`u:<13 digits>`), so every build
// dirtied the committed, embedded dist with a one-line diff carrying no
// information (TODO 2026-09-09; nothing in web/src reads the value). Replace
// it with `u:0` AFTER the build. Bytes, not text: the file carries a NUL
// byte that .gitattributes documents, and a text round-trip must not touch it.
import { readFileSync, writeFileSync } from "node:fs";

const path = new URL("../dist/client/_shell.html", import.meta.url);
const buf = readFileSync(path);
const text = buf.toString("latin1"); // one char per byte: offsets are byte offsets
const all = [...text.matchAll(/u:(\d{13})/g)];
if (all.length !== 1) {
  // Exactly one: zero means the prerender output changed shape; more than
  // one means this rewrite would touch something it does not understand
  // (security review of the close-out's phase 2, low).
  console.error(`normalize-shell: expected exactly one u:<13 digits> timestamp in _shell.html, found ${all.length}`);
  process.exit(1);
}
const m = all[0];
// `u:0`, not a zero-padded 13-digit field: the payload is a JavaScript
// object literal inside a <script>, where 0000000000000 is a legacy octal
// literal and a SyntaxError in strict mode (security review 2026-09-10).
const out = Buffer.from(text.slice(0, m.index) + "u:0" + text.slice(m.index + m[0].length), "latin1");
writeFileSync(path, out);
console.log(`normalize-shell: u:${m[1]} -> u:0 (${buf.length} -> ${out.length} bytes)`);
