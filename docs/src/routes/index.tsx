import { createFileRoute, Link } from "@tanstack/react-router";

export const Route = createFileRoute("/")({
	component: Home,
});

const docs: { title: string; description: string; slug: string }[] = [
	{
		title: "Overview",
		description: "What the backend does",
		slug: "",
	},
	{
		title: "Setup",
		description: "Local setup and configuration",
		slug: "setup",
	},
	{
		title: "Architecture",
		description: "Layers, modules, and request flow",
		slug: "architecture",
	},
	{
		title: "Services",
		description: "Domain services and repositories",
		slug: "services",
	},
	{
		title: "Event System",
		description: "In-memory pub/sub events",
		slug: "events",
	},
	{
		title: "Database",
		description: "SQLite locally, D1 in production",
		slug: "database",
	},
	{
		title: "Deployment",
		description: "Lambda deploys and environments",
		slug: "deployment",
	},
];

function Home() {
	return (
		<main className="bg-background text-foreground flex min-h-screen flex-col pb-20 antialiased">
			<div className="mx-auto w-full max-w-173 flex-1 px-4 pt-14">
				<section aria-label="KR Crackers backend">
					<div className="flex flex-col items-start">
						<h1>
							<img
								src="/kr-crackers.svg"
								alt="KR Crackers"
								className="h-8 w-auto dark:invert"
							/>
						</h1>
						<p className="mt-4 text-sm leading-relaxed text-muted-foreground sm:text-base">
							A Go-powered e-commerce backend for order management, product
							catalog, and admin dashboard, with dual SQLite/D1 storage and
							event-driven services under the hood.
						</p>
						<div className="mt-6 flex w-full flex-col gap-3 sm:w-auto sm:flex-row sm:items-center">
							<Link
								to="/docs/$"
								params={{ _splat: "" }}
								className="bg-primary text-primary-foreground w-fit rounded-lg px-5 py-2 text-sm font-medium transition-transform duration-150 ease-out hover:opacity-90 active:scale-[0.96]"
							>
								Get started
							</Link>
							<a
								href="https://github.com/thaletto/krcrackers-go"
								target="_blank"
								rel="noopener noreferrer"
								className="border-border bg-background w-fit rounded-lg border px-5 py-2 text-sm font-medium transition-colors hover:bg-accent hover:text-accent-foreground"
							>
								GitHub
							</a>
						</div>
					</div>
				</section>
				<section className="mt-16 sm:mt-24">
					<h2 className="section-tag-ruled text-sm font-medium sm:text-base">
						<span>Docs</span>
						<span className="section-tag-rule" aria-hidden="true" />
					</h2>
					<ul className="mt-2 flex flex-col">
						{docs.map((d) => (
							<li key={d.slug}>
								<Link
									to="/docs/$"
									params={{ _splat: d.slug }}
									className="group flex flex-wrap items-center gap-2 py-3.5"
								>
									<span className="min-w-0 text-sm font-medium text-foreground sm:text-base">
										{d.title}
									</span>
									<span
										aria-hidden="true"
										className="hidden shrink-0 text-sm text-muted-foreground sm:inline"
									>
										/
									</span>
									<span className="min-w-0 basis-full truncate text-sm text-muted-foreground transition-colors duration-150 ease-out group-hover:text-foreground sm:basis-auto sm:flex-1">
										{d.description}
									</span>
								</Link>
							</li>
						))}
						<li>
							<a
								href="/openapi.json"
								className="group flex flex-wrap items-center gap-2 py-3.5"
							>
								<span className="min-w-0 text-sm font-medium text-foreground sm:text-base">
									API Reference
								</span>
								<span
									aria-hidden="true"
									className="hidden shrink-0 text-sm text-muted-foreground sm:inline"
								>
									/
								</span>
								<span className="min-w-0 basis-full truncate text-sm text-muted-foreground transition-colors duration-150 ease-out group-hover:text-foreground sm:basis-auto sm:flex-1">
									Raw OpenAPI JSON spec
								</span>
							</a>
						</li>
					</ul>
				</section>
			</div>
		</main>
	);
}
