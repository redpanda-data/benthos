// Bundle the Bloblang V2 interpreter as a single ES module for browser use.
import { build } from "esbuild";
import { copyFileSync } from "fs";

await build({
  entryPoints: ["src/index.ts"],
  bundle: true,
  format: "esm",
  outfile: "dist/bloblang2.mjs",
  target: "es2022",
  minify: false,
  sourcemap: true,
});

// Copy to the demo directory for the Go server to embed.
copyFileSync("dist/bloblang2.mjs", "../demo/bloblang2.mjs");
copyFileSync("dist/bloblang2.mjs.map", "../demo/bloblang2.mjs.map");

console.log("Bundled to dist/bloblang2.mjs and copied to demo/");
