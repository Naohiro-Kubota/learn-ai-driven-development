import { execFileSync } from "node:child_process";
import process from "node:process";

export function findUnformattedFiles(files, runGofmt) {
	if (files.length === 0) return [];
	return runGofmt(files).split("\n").filter(Boolean);
}

function trackedGoFiles() {
	return execFileSync("git", ["ls-files", "-z", "--", "*.go"], {
		encoding: "utf8",
	})
		.split("\0")
		.filter(Boolean);
}

function runGofmt(files) {
	return execFileSync("gofmt", ["-l", ...files], { encoding: "utf8" });
}

const files = findUnformattedFiles(trackedGoFiles(), runGofmt);
if (files.length > 0) {
	process.stderr.write(`Run gofmt on:\n${files.join("\n")}\n`);
	process.exitCode = 1;
}
