import { defineConfig } from "vitest/config";

export default defineConfig({
  test: {
    // The picker is DOM code, so the suite runs against jsdom. The pure
    // formatting tests do not care either way.
    environment: "jsdom",
    include: ["src/**/*.test.js"],
  },
});
