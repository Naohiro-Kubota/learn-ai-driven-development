import assert from "node:assert/strict";
import test from "node:test";

import { findUnformattedFiles } from "./check-gofmt.mjs";

test("returns no paths when gofmt reports no unformatted files", () => {
	assert.deepEqual(
		findUnformattedFiles(["internal/example.go"], () => ""),
		[],
	);
});

test("reports paths printed by gofmt -l", () => {
	assert.deepEqual(
		findUnformattedFiles(
			["internal/example.go"],
			() => "internal/example.go\n",
		),
		["internal/example.go"],
	);
});
