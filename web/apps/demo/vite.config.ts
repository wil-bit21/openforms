import { defineConfig } from "vitest/config";
import react from "@vitejs/plugin-react";
import { fileURLToPath } from "node:url";

// Repo root: the demo imports ../../../examples/openforms/*.yaml via ?raw.
const repoRoot = fileURLToPath(new URL("../../..", import.meta.url));
const api = "http://localhost:8080";

export default defineConfig({
  base: "/_app/demo/",
  plugins: [react()],
  server: {
    port: 5175,
    fs: { allow: [repoRoot] },
    proxy: {
      "/api": api,
      "/healthz": api,
      "/embed.js": api,
      "/f/": api,
      "/s/": api,
      "/admin": api,
      "/_app/hosted": api,
      "/_app/admin": api,
    },
  },
  test: {
    environment: "jsdom",
    environmentOptions: { jsdom: { url: "http://localhost:3000/demo" } },
    setupFiles: ["./src/test/setup.ts"],
    css: false,
  },
});
