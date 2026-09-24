import { test } from "node:test";
import assert from "node:assert/strict";
import { mkdtempSync, mkdirSync, writeFileSync, existsSync, readFileSync, readdirSync } from "node:fs";
import { tmpdir } from "node:os";
import { join } from "node:path";
import { copyDist } from "./copy-dist.mjs";

function fixture() {
  const root = mkdtempSync(join(tmpdir(), "of-copy-"));
  const web = join(root, "web");
  const out = join(root, "src", "openforms", "server", "ui");
  mkdirSync(join(web, "apps", "hosted", "dist", "assets"), { recursive: true });
  writeFileSync(join(web, "apps", "hosted", "dist", "index.html"), "<html>hosted</html>");
  writeFileSync(join(web, "apps", "hosted", "dist", "assets", "app.js"), "console.log(1)");
  mkdirSync(join(web, "apps", "admin"), { recursive: true }); // app without a build
  mkdirSync(join(web, "packages", "embed", "dist"), { recursive: true });
  writeFileSync(join(web, "packages", "embed", "dist", "embed.js"), "/*embed*/");
  mkdirSync(join(out, "stale"), { recursive: true });
  writeFileSync(join(out, "stale", "old.html"), "old");
  writeFileSync(join(out, ".gitkeep"), "");
  return { web, out };
}

test("copies built apps and embed.js, keeps .gitkeep, removes stale output", () => {
  const { web, out } = fixture();
  const copied = copyDist(web, out);
  assert.deepEqual(copied, ["hosted", "embed"]);
  assert.equal(readFileSync(join(out, "hosted", "index.html"), "utf8"), "<html>hosted</html>");
  assert.ok(existsSync(join(out, "hosted", "assets", "app.js")));
  assert.equal(readFileSync(join(out, "embed", "embed.js"), "utf8"), "/*embed*/");
  assert.ok(existsSync(join(out, ".gitkeep")));
  assert.ok(!existsSync(join(out, "stale")));
  assert.ok(!existsSync(join(out, "admin")));
});

test("creates the output directory when missing", () => {
  const root = mkdtempSync(join(tmpdir(), "of-copy-empty-"));
  const out = join(root, "dist");
  assert.deepEqual(copyDist(join(root, "web"), out), []);
  assert.deepEqual(readdirSync(out), []);
});
