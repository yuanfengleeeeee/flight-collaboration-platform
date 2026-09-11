import { defineConfig, loadEnv } from "vite";
import react from "@vitejs/plugin-react";

export default defineConfig(({ mode }) => {
  const env = loadEnv(mode, "../..", "VITE_");
  return {
    plugins: [react()],
    server: {
      port: 4174,
      proxy: { "/core-api": { target: env.VITE_CORE_API_UPSTREAM ?? "http://127.0.0.1:8081", changeOrigin: true, rewrite: (path) => path.replace(/^\/core-api/, "") } },
    },
  };
});
