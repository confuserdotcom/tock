import path from "node:path";
import { defineConfig } from "vite";
import react from "@vitejs/plugin-react";

// The build goes straight into the Go package, with fixed names: the Go
// server embeds internal/web/static and its tests name app.js and app.css.
export default defineConfig({
  plugins: [react()],
  resolve: { alias: { "@": path.resolve(__dirname, "src") } },
  base: "./",
  // npm run dev: the API comes from the demo server (see docs/guide.md).
  server: { proxy: { "/api": "http://127.0.0.1:7788" } },
  build: {
    outDir: "../internal/web/static",
    emptyOutDir: true,
    assetsInlineLimit: 0,
    cssCodeSplit: false,
    modulePreload: false,
    chunkSizeWarningLimit: 1200, // served from the machine itself, not the internet
    rollupOptions: {
      output: {
        entryFileNames: "app.js",
        inlineDynamicImports: true,
        assetFileNames: (a) => (a.name?.endsWith(".css") ? "app.css" : "fonts/[name][extname]"),
      },
    },
  },
  test: { environment: "node", include: ["src/**/*.test.ts"] },
});
