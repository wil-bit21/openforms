import react from "@vitejs/plugin-react";
import { defineConfig } from "vite";

export default defineConfig({
  base: "/_app/hosted/",
  plugins: [react()],
  server: {
    port: 5174,
    proxy: { "/api": "http://localhost:8080", "/healthz": "http://localhost:8080" },
  },
  build: { outDir: "dist", emptyOutDir: true },
});
