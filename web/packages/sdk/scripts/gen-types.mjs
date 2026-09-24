// Generates src/generated/*.ts from ../../../schemas/*.schema.json.
// Usage: node scripts/gen-types.mjs          (write files)
//        node scripts/gen-types.mjs --check  (exit 1 if committed files are stale)
import { compile } from "json-schema-to-typescript";
import { existsSync, mkdirSync, readFileSync, writeFileSync } from "node:fs";
import { dirname, join, resolve } from "node:path";
import { fileURLToPath } from "node:url";

const here = dirname(fileURLToPath(import.meta.url));
const schemasDir = resolve(here, "../../../../schemas");
const outDir = resolve(here, "../src/generated");
const check = process.argv.includes("--check");

const targets = [
  { schema: "form.schema.json", typeName: "FormDefinition", out: "form.ts" },
  { schema: "workflow.schema.json", typeName: "WorkflowDefinition", out: "workflow.ts" },
];

let stale = false;
mkdirSync(outDir, { recursive: true });

for (const t of targets) {
  const schema = JSON.parse(readFileSync(join(schemasDir, t.schema), "utf8"));
  // The root type name must be stable regardless of the schema's own title/$id.
  delete schema.title;
  delete schema.$id;
  const ts = await compile(schema, t.typeName, {
    cwd: schemasDir,
    additionalProperties: false,
    unreachableDefinitions: false,
    bannerComment: `/* eslint-disable */\n/**\n * GENERATED from schemas/${t.schema} by web/packages/sdk/scripts/gen-types.mjs.\n * Do not edit by hand: run \`pnpm -C web/packages/sdk gen\`.\n */`,
    style: { semi: true, singleQuote: false, trailingComma: "all", printWidth: 100 },
  });
  const file = join(outDir, t.out);
  if (check) {
    const current = existsSync(file) ? readFileSync(file, "utf8") : "";
    if (current !== ts) {
      console.error(`stale: src/generated/${t.out} — run \`pnpm -C web/packages/sdk gen\``);
      stale = true;
    }
  } else {
    writeFileSync(file, ts);
    console.log(`wrote src/generated/${t.out}`);
  }
}

if (stale) process.exit(1);
