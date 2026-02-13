import type { ReactNode } from "react";
import { MarketingHeader } from "./marketing-header";
import { MarketingFooter } from "./marketing-footer";

type MarketingShellProps = {
	title: string;
	description: string;
	children: ReactNode;
};

export function MarketingShell({
	title,
	description,
	children,
}: MarketingShellProps) {
	return (
		<div className="relative flex min-h-screen flex-col bg-railway-bg text-railway-text font-display antialiased overflow-x-hidden">
			<div className="fixed inset-0 bg-square-grid bg-grid-pattern pointer-events-none z-0 opacity-[0.8] grid-mask" />

			<MarketingHeader />

			<main className="relative z-10 mx-auto w-full max-w-5xl flex-1 px-6 pb-24 pt-32">
				<div className="mb-10 max-w-3xl space-y-4">
					<h1 className="text-4xl md:text-5xl font-[200] tracking-tight text-off-white leading-[1.1]">
						{title}
					</h1>
					<p className="text-lg text-railway-muted font-light leading-relaxed">
						{description}
					</p>
				</div>
				<div className="space-y-6">{children}</div>
			</main>

			<MarketingFooter />
		</div>
	);
}
