import { defineConfig } from "vitest/config";

export default defineConfig({
  test: {
    testTimeout: 30_000,
    pool: "forks",
    poolOptions: {
      forks: {
        execArgv: ["--stack-size=65536"],
      },
    },
  },
});
