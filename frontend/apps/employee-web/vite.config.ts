import { defineConfig, loadEnv } from "vite";
import react from "@vitejs/plugin-react";

export default defineConfig(({ mode }) => {
  const env = loadEnv(mode, "../..", "VITE_");
  return {
    plugins: [react()],
    server: {
      port: 4175,
      // Keep the browser Host so Edge's same-origin WebSocket check sees the
      // external employee-web origin instead of the local upstream address.
      proxy: { "/edge-api": { target: env.VITE_EDGE_API_UPSTREAM ?? "http://127.0.0.1:8082", changeOrigin: false, ws: true, rewrite: (path) => path.replace(/^\/edge-api/, "") } },
    },
  };
});
