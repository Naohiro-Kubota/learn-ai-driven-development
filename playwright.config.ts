import { defineConfig } from "@playwright/test";

declare const process: {
	env: { E2E_PLAYWRIGHT_OUTPUT_DIR?: string };
};

const outputDir = process.env.E2E_PLAYWRIGHT_OUTPUT_DIR;
if (!outputDir) throw new Error("Run browser tests through pnpm run test:e2e");

export default defineConfig({
	testDir: "./frontend/e2e",
	outputDir,
	workers: 1,
	retries: 0,
	reporter: "list",
	use: {
		baseURL: "http://127.0.0.1:5173",
		trace: "off",
		video: "off",
		screenshot: "off",
	},
});
