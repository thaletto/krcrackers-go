import { createFileRoute } from "@tanstack/react-router";
import spec from "../../openapi/openapi.json";

export const Route = createFileRoute("/openapi.json")({
	server: {
		handlers: {
			GET: async () =>
				new Response(JSON.stringify(spec), {
					headers: {
						"Content-Type": "application/json",
					},
				}),
		},
	},
});
