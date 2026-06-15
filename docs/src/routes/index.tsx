import { createFileRoute, Link } from "@tanstack/react-router";
import { HomeLayout } from "fumadocs-ui/layouts/home";
import { baseOptions } from "@/lib/layout.shared";
import { Layers, Radio, Database, Box, Rocket, Code } from "lucide-react";

export const Route = createFileRoute("/")({
	component: Home,
});

const features = [
	{
		icon: Layers,
		title: "Layered Architecture",
		description:
			"Clean separation between handlers, repositories, and database. Repository pattern with dependency injection.",
		link: "architecture",
	},
	{
		icon: Radio,
		title: "Event-Driven",
		description:
			"In-memory pub/sub bus decouples services. Search syncs via Meilisearch, notifications via WhatsApp.",
		link: "events",
	},
	{
		icon: Database,
		title: "Dual Database",
		description:
			"SQLite in development, Cloudflare D1 in production. Same interface, zero code changes between environments.",
		link: "database",
	},
	{
		icon: Box,
		title: "Domain Services",
		description:
			"Auth, Products, Orders, Customers, Invoices. Each with its own repository, handlers, and event publishing.",
		link: "services",
	},
	{
		icon: Code,
		title: "REST API",
		description:
			"33 endpoints with JWT auth, Google OAuth, file uploads, and interactive API playground.",
		link: "architecture",
	},
	{
		icon: Rocket,
		title: "Dual Deploy",
		description:
			"Standalone HTTP server or AWS Lambda. Same handler, same code. ~200ms cold start on arm64.",
		link: "deployment",
	},
];

function Home() {
	return (
		<HomeLayout {...baseOptions()}>
			<div className="flex flex-col flex-1">
				{/* Hero with Zed-style background pattern */}
				<section className="relative overflow-hidden">
					{/* Background pattern */}
					<div className="absolute inset-0 -z-10">
						{/* Base gradient */}
						<div className="absolute inset-0 bg-linear-to-b from-fd-background via-fd-background to-fd-muted/30" />

						{/* Grid pattern */}
						<svg className="absolute inset-0 h-full w-full opacity-[0.03] dark:opacity-[0.05]">
							<defs>
								<pattern
									id="grid"
									width="40"
									height="40"
									patternUnits="userSpaceOnUse"
								>
									<path
										d="M 40 0 L 0 0 0 40"
										fill="none"
										stroke="currentColor"
										strokeWidth="1"
									/>
								</pattern>
							</defs>
							<rect width="100%" height="100%" fill="url(#grid)" />
						</svg>
					</div>

					<div className="mx-auto max-w-4xl px-6 pt-24 pb-16 text-center">
						<h1 className="font-bold text-5xl md:text-6xl tracking-tight mb-4">
							KR Crackers
						</h1>
						<p className="text-fd-muted-foreground text-lg max-w-xl mx-auto mb-8">
							Go-powered e-commerce backend with order lifecycle management,
							product catalog, and admin dashboard.
						</p>
						<div className="flex gap-3 justify-center">
							<Link
								to="/docs/$"
								params={{ _splat: "" }}
								className="px-5 py-2.5 rounded-lg bg-fd-primary text-fd-primary-foreground font-medium text-sm"
							>
								Get Started
							</Link>
							<a
								href="https://github.com/thaletto/krcrackers-go"
								className="px-5 py-2.5 rounded-lg border bg-fd-background font-medium text-sm"
								target="_blank"
								rel="noopener noreferrer"
							>
								GitHub
							</a>
						</div>
					</div>
				</section>

				{/* Features grid */}
				<section className="border-t">
					<div className="mx-auto max-w-5xl grid grid-cols-1 md:grid-cols-3 gap-px bg-fd-border">
						{features.map((f) => (
							<Link
								key={f.title}
								to="/docs/$"
								params={{ _splat: f.link }}
								className="group p-8 bg-fd-background hover:bg-fd-muted/50 transition-colors"
							>
								<f.icon className="w-5 h-5 mb-3 text-fd-muted-foreground" />
								<h3 className="font-semibold mb-1">{f.title}</h3>
								<p className="text-sm text-fd-muted-foreground leading-relaxed">
									{f.description}
								</p>
							</Link>
						))}
					</div>
				</section>
			</div>
		</HomeLayout>
	);
}
