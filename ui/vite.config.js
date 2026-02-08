import path from "node:path";
import react from "@vitejs/plugin-react";
import { defineConfig } from "vite";
export default defineConfig({
    plugins: [react()],
    build: {
        outDir: path.resolve(import.meta.dirname, "dist/public"),
        emptyOutDir: true,
    },
    server: {
        port: 3000,
        strictPort: false,
        host: true,
        proxy: {
            "/api": {
                target: "http://localhost:3001",
                changeOrigin: true,
            },
            "/ws": {
                target: "ws://localhost:3001",
                ws: true,
            },
        },
    },
});
