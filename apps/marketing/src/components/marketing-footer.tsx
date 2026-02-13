import Link from "next/link";
import { LogoMark, Separator } from "@spacescale/ui";

const footerLinks = {
	product: [
		{ label: "Features", href: "/" },
		{ label: "Pricing", href: "/pricing" },
	],
	company: [
		{ label: "About", href: "/" },
		{ label: "Careers", href: "/careers" },
	],
	legal: [
		{ label: "Privacy", href: "/privacy" },
		{ label: "Terms", href: "/terms" },
	],
};

export function MarketingFooter() {
	const currentYear = new Date().getFullYear();

	return (
		<footer className="border-t hairline-border border-white/5 bg-[#0b0d14] py-10">
			<div className="mx-auto flex max-w-7xl flex-col items-start justify-between gap-8 px-6 md:flex-row">
				<div>
					<div className="mb-4 flex items-center gap-2">
						<LogoMark className="scale-90 opacity-80" />
						<span className="text-sm font-light text-off-white">
							SpaceScale
						</span>
					</div>
					<p className="max-w-xs text-xs font-light text-railway-muted">
						Infrastructure for modern engineering teams. <br />
						Ontario, Canada.
					</p>
				</div>
				<div className="flex gap-12">
					{Object.entries(footerLinks).map(([section, links]) => (
						<div key={section} className="flex flex-col gap-3">
							<span className="text-[10px] font-semibold uppercase tracking-wider text-off-white opacity-50">
								{section}
							</span>
							{links.map((link) => (
								<Link
									key={link.href + link.label}
									className="text-xs text-railway-muted transition-colors hover:text-off-white"
									href={link.href}
								>
									{link.label}
								</Link>
							))}
						</div>
					))}
				</div>
			</div>
			<Separator className="mx-auto mt-8 max-w-7xl bg-white/5" />
			<div className="mx-auto flex max-w-7xl items-center justify-between px-6 pt-6">
				<p className="text-[10px] uppercase text-railway-muted">
					&copy; {currentYear} SpaceScale Inc.
				</p>
				<div className="flex items-center gap-2">
					<span className="size-1.5 rounded-full bg-indigo-500 shadow-[0_0_5px_rgba(99,102,241,0.8)]" />
					<span className="text-[10px] font-mono uppercase text-railway-muted">
						Systems Normal
					</span>
				</div>
			</div>
		</footer>
	);
}
