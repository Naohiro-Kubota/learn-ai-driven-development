import { expect, test } from "vitest";

import { findUnformattedFiles } from "./check-gofmt.mjs";

test("returns no paths when gofmt reports no unformatted files", () => {
	expect(findUnformattedFiles(["internal/example.go"], () => "")).toEqual([]);
});

test("reports paths printed by gofmt -l", () => {
	expect(
		findUnformattedFiles(
			["internal/example.go"],
			() => "internal/example.go\n",
		),
	).toEqual(["internal/example.go"]);
});
