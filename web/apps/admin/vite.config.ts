import { defineConfig } from "vitest/config";
import react from "@vitejs/plugin-react";
import { fileURLToPath } from "node:url";

const sdkSrc = fileURLToPath(new URL("../../packages/sdk/src/index.ts", import.meta.url));

export default defineConfig({
  base: "/_app/admin/",
  plugins: [react()],
  resolve: { alias: { "@openforms/sdk": sdkSrc } },
  server: {
    port: 5174,
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
