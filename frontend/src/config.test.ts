import { describe, expect, it } from "vitest";
import { loginUrl, parseApiOrigin } from "./config";

describe("loginUrl", () => {
	it.each([
		["https://api.example.test", "https://api.example.test/auth/oidc/login"],
		["http://127.0.0.1:8080", "http://127.0.0.1:8080/auth/oidc/login"],
	])("uses the fixed login path for %s", (value, expected) => {
		const destination = loginUrl(parseApiOrigin(value));
		expect(destination).toBe(expected);
		expect(new URL(destination).search).toBe("");
		expect(new URL(destination).hash).toBe("");
	});
});

describe("parseApiOrigin", () => {
	it.each([
		["https://api.example.test", "https://api.example.test"],
		["https://api.example.test/", "https://api.example.test"],
		["http://127.0.0.1:8080", "http://127.0.0.1:8080"],
		["http://localhost:8080", "http://localhost:8080"],
		["http://[::1]:8080", "http://[::1]:8080"],
	])("accepts %s as an API origin", (value, expected) => {
		const origin = parseApiOrigin(value);
		expect(origin.origin).toBe(expected);
		expect(origin.pathname).toBe("/");
		expect(origin.search).toBe("");
		expect(origin.hash).toBe("");
	});

	it.each([
		undefined,
		"",
		" /api ",
		"/api",
		"https://u@api.example.test",
		"https://api.example.test/path",
		"https://api.example.test/a/../",
		"https://api.example.test/./",
		"https://api.example.test/%2e/",
		"https://api.example.test\\a\\..\\",
		"https://api.example.test/?x=1",
		"https://api.example.test/#x",
		"http://api.example.test",
		"http://192.168.1.10:8080",
		" https://api.example.test",
		"https://api.example.test ",
	])("rejects invalid VITE_API_ORIGIN %s", (value) => {
		expect(() => parseApiOrigin(value)).toThrow(/VITE_API_ORIGIN/);
	});
});
