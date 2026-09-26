import { defineConfig } from "vite";
import react from "@vitejs/plugin-react";

export default defineConfig({
  plugins: [react()],
  server: {
    // Real-API mode: the backend serves /auth.v1.PlatformService/* on :8081.
    // Proxying keeps the console same-origin in dev (no CORS needed).
    proxy: {
      "/auth.v1": {
        target: "http://localhost:8081",
        changeOrigin: true,
      },
    },
  },
});
