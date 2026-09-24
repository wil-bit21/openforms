import { defineConfig } from "vitest/config";
import react from "@vitejs/plugin-react";
import { fileURLToPath } from "node:url";

const sdkSrc = fileURLToPath(new URL("../../packages/sdk/src/index.ts", import.meta.url));
const reactSrc = fileURLToPath(new URL("../../packages/react/src", import.meta.url));
const repoRoot = fileURLToPath(new URL("../../..", import.meta.url));
const schemasDir = fileURLToPath(new URL("../../../schemas", import.meta.url));

export default defineConfig({
  base: "/_app/admin/",
  plugins: [react()],
  resolve: {
    alias: [
      { find: "@openforms/sdk", replacement: sdkSrc },
      { find: /^@openforms\/react\/styles\.css$/, replacement: `${reactSrc}/styles.css` },
      { find: /^@openforms\/react$/, replacement: `${reactSrc}/index.ts` },
      { find: "@schemas", replacement: schemasDir },
    ],
  },
  server: {
    port: 5174,
    fs: { allow: [repoRoot] },
    proxy: {
      "/api": "http://localhost:8080",
      "/healthz": "http://localhost:8080",
    },
  },
  test: {
    environment: "jsdom",
    setupFiles: ["./src/test/setup.ts"],
    css: false,
  },
});
