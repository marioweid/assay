import { fileURLToPath, URL } from "node:url";

import tailwindcss from "@tailwindcss/vite";
import react from "@vitejs/plugin-react";
import { defineConfig } from "vite";

const backend = "http://localhost:8080";

export default defineConfig({
  plugins: [react(), tailwindcss()],
  resolve: {
    alias: {
      "@": fileURLToPath(new URL("./src", import.meta.url)),
    },
  },
  build: {
    outDir: fileURLToPath(new URL("../assayd/internal/ui/dist", import.meta.url)),
    emptyOutDir: true,
  },
  server: {
    proxy: {
      "/v1": backend,
      "/openapi.json": backend,
      "/docs": backend,
      "/healthz": backend,
      "/readyz": backend,
    },
  },
});
