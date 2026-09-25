import react from "@vitejs/plugin-react";
import { defineConfig } from "vitest/config";

export default defineConfig({
	root: "frontend",
	plugins: [react()],
	test: {
		root: ".",
		environment: "jsdom",
		include: [
			"frontend/src/**/*.test.{ts,tsx}",
			"scripts/check-gofmt.test.mjs",
		],
		setupFiles: ["./frontend/src/test/setup.ts"],
	},
});
