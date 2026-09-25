import { defineConfig } from "vite";

export default defineConfig({
  build: {
    lib: { entry: "src/index.ts", name: "OpenFormsEmbed", formats: ["iife"], fileName: () => "embed.js" },
    outDir: "dist",
    emptyOutDir: true,
    target: "es2019",
    minify: true,
  },
});
