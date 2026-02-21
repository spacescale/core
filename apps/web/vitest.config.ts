import { defineConfig } from "vitest/config";
import react from "@vitejs/plugin-react";
import path from "path";

export default defineConfig({
  plugins: [react()],
  test: {
    environment: "jsdom",
    globals: true,
    setupFiles: ["./src/test/setup.ts"],
    include: ["src/**/*.{test,spec}.{js,mjs,cjs,ts,mts,cts,jsx,tsx}"],
    coverage: {
      reporter: ["text", "json", "html"],
      exclude: ["node_modules/", "src/test/setup.ts"],
    },
  },
  resolve: {
    alias: [
      {
        find: /^next-auth\/providers\/github$/,
        replacement: path.resolve(
          __dirname,
          "./src/test/mocks/next-auth-provider-github.ts",
        ),
      },
      {
        find: /^next-auth\/providers\/google$/,
        replacement: path.resolve(
          __dirname,
          "./src/test/mocks/next-auth-provider-google.ts",
        ),
      },
      {
        find: /^next-auth$/,
        replacement: path.resolve(__dirname, "./src/test/mocks/next-auth.ts"),
      },
      {
        find: "@",
        replacement: path.resolve(__dirname, "./src"),
      },
    ],
  },
});
