import { defineConfig } from "vite";
import react from "@vitejs/plugin-react-swc";
import path from "node:path";

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
        target: "http://localhost:3100",
        changeOrigin: true,
      },
    },
  },
});
