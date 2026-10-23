import { defineConfig } from "vite";
import react from "@vitejs/plugin-react";

// Docker Desktop loads the UI from the extension image with a file-like
// origin, so asset paths must be relative.
export default defineConfig({
  plugins: [react()],
  base: "./",
  build: {
    outDir: "build",
    chunkSizeWarningLimit: 1024,
  },
  server: {
    port: 3000,
    strictPort: true,
  },
});
