export function parseApiOrigin(value: string | undefined): URL {
	if (
		!value ||
		value.trim() !== value ||
		!/^https?:\/\/[^/?#\\]+\/?$/i.test(value)
	) {
		throw new Error("VITE_API_ORIGIN must be an absolute HTTP(S) origin");
	}

	let url: URL;
	try {
		url = new URL(value);
	} catch {
		throw new Error("VITE_API_ORIGIN must be an absolute HTTP(S) origin");
	}

	const loopback = ["localhost", "127.0.0.1", "[::1]"].includes(url.hostname);
	if (
		(url.protocol !== "https:" && !(url.protocol === "http:" && loopback)) ||
		url.username ||
		url.password ||
		url.pathname !== "/" ||
		url.search ||
		url.hash ||
		url.href !== `${url.origin}/`
	) {
		throw new Error("VITE_API_ORIGIN must be an absolute HTTP(S) origin");
	}

	return url;
}

export function loginUrl(apiOrigin: URL): string {
	return `${apiOrigin.origin}/auth/oidc/login`;
}
