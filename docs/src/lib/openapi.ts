import { createOpenAPI } from "fumadocs-openapi/server";

export const openapi = createOpenAPI({
	input: ["./openapi/openapi.json"],
	proxyUrl: "/api/proxy",
});
