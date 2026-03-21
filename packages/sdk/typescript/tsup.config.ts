import { defineConfig } from "tsup";

export default defineConfig({
  entry: {
    index: "src/index.ts",
    node: "src/node.ts",
  },
  format: ["esm", "cjs"],
  dts: true,
  splitting: false,
  sourcemap: true,
  clean: true,
  target: "es2020",
});
