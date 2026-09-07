import { defineConfig } from "@hey-api/openapi-ts";

export default defineConfig({
  input: "./openapi.json",
  output: {
    header: (context) => [
      "// @ts-nocheck -- Hey API 0.99 templates conflict with exactOptionalPropertyTypes.",
      ...context.defaultValue,
    ],
    path: "./src/api/generated",
    postProcess: ["oxfmt"],
  },
  plugins: ["@hey-api/client-fetch", "@hey-api/sdk"],
});
