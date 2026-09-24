// Copies web build outputs into src/openforms/server/ui, where the Python server serves
// them from (and the wheel packages them).
import { cpSync, existsSync, mkdirSync, readdirSync, rmSync } from "node:fs";
import { dirname, join, resolve } from "node:path";
import { fileURLToPath, pathToFileURL } from "node:url";

export function copyDist(webDir, outDir) {
  mkdirSync(outDir, { recursive: true });
  for (const entry of readdirSync(outDir)) {
    if (entry === ".gitkeep") continue;
    rmSync(join(outDir, entry), { recursive: true, force: true });
  }

  const copied = [];
  const appsDir = join(webDir, "apps");
  if (existsSync(appsDir)) {
    for (const name of readdirSync(appsDir).sort()) {
      const dist = join(appsDir, name, "dist");
      if (!existsSync(dist)) continue;
      cpSync(dist, join(outDir, name), { recursive: true });
      copied.push(name);
    }
  }

  const embed = join(webDir, "packages", "embed", "dist", "embed.js");
  if (existsSync(embed)) {
    mkdirSync(join(outDir, "embed"), { recursive: true });
    cpSync(embed, join(outDir, "embed", "embed.js"));
    copied.push("embed");
  }
  return copied;
}

const invokedDirectly =
  process.argv[1] !== undefined && pathToFileURL(resolve(process.argv[1])).href === import.meta.url;

if (invokedDirectly) {
  const webDir = resolve(dirname(fileURLToPath(import.meta.url)), "..");
  const outDir = resolve(webDir, "..", "src", "openforms", "server", "ui");
  const copied = copyDist(webDir, outDir);
  console.log(`copy-dist: ${copied.length ? copied.join(", ") : "nothing to copy"} → ${outDir}`);
}
