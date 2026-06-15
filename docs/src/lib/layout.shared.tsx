import type { BaseLayoutProps } from "fumadocs-ui/layouts/shared";
import { gitConfig } from "./shared";
import { Flame } from "lucide-react";

export function baseOptions(): BaseLayoutProps {
	return {
		nav: {
			// JSX supported
			title: (
				<span className="flex items-center gap-1">
					<Flame /> KR Crackers Docs
				</span>
			),
		},
		githubUrl: `https://github.com/${gitConfig.user}/${gitConfig.repo}`,
	};
}
