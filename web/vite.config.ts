import { defineConfig } from "vitest/config";
import react from "@vitejs/plugin-react";
import path from "node:path";

export default defineConfig({
  plugins: [react()],
  resolve: {
    alias: { "@": path.resolve(__dirname, "src") },
  },
  test: {
    environment: "jsdom",
    setupFiles: "./src/test/setup.ts",
    fileParallelism: false,
    pool: "forks",
  },
  server: {
    port: 5173,
    proxy: {
      "/api": "http://127.0.0.1:8443",
    },
  },
  worker: { format: "es" },
});
