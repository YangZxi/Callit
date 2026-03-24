import { defineConfig } from "vite";
import react from "@vitejs/plugin-react-swc";
import path from "node:path";

const apiProxyTarget = process.env.VITE_API_PROXY_TARGET || "http://localhost:3100";

export default defineConfig({
  base: "/admin/",
  build: {
    outDir: "dist",
  },
  plugins: [react()],
  resolve: {
    alias: {
      "@": path.resolve(__dirname, "src"),
    },
  },
  server: {
    port: 3180,
    proxy: {
      "/admin/api": {
        target: apiProxyTarget,
        changeOrigin: true,
      },
      "/admin0823/api": {
        target: "https://c.xiaosm.cn",
        changeOrigin: true,
      },
    },
  },
});
