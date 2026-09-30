import type { BaseLayoutProps } from "fumadocs-ui/layouts/shared";
import { gitConfig } from "./shared";

export function baseOptions(): BaseLayoutProps {
	return {
		nav: {
			// JSX supported
			title: (
				<img
					src="/kr-crackers.svg"
					alt="KR Crackers"
					className="h-6 w-auto dark:invert"
				/>
			),
		},
		githubUrl: `https://github.com/${gitConfig.user}/${gitConfig.repo}`,
	};
}
