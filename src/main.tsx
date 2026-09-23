import { createRoot } from "react-dom/client";
import { createApiClient } from "./api/client";
import { App } from "./app";
import { loginUrl, parseApiOrigin } from "./config";
import "./styles.css";

const apiOrigin = parseApiOrigin(import.meta.env.VITE_API_ORIGIN);
const client = createApiClient(apiOrigin);
const login = () => {
	window.location.assign(loginUrl(apiOrigin));
};

const root = document.getElementById("root");
if (!root) {
	throw new Error("Root element is missing");
}

createRoot(root).render(<App client={client} login={login} />);
