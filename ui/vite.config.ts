import { defineConfig } from "vite";

// No React plugin is required: Vite's esbuild transform handles TSX with the
// automatic JSX runtime. The built viewer stays local and dependency-free at
// runtime.
export default defineConfig({
  base: "./",
  esbuild: { jsx: "automatic" },
  build: { outDir: "dist", emptyOutDir: true },
});
