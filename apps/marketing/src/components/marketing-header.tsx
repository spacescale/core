import Link from "next/link";
import { LogoMark } from "@spacescale/ui";
import { getDashboardUrls } from "@/lib/urls";

export function MarketingHeader() {
	const { login: loginHref, deploy: deployHref } = getDashboardUrls();

	return (
		<header className="fixed top-0 z-50 w-full border-b hairline-border border-railway-border/40 bg-railway-bg/70 backdrop-blur-md">
			<div className="mx-auto flex h-16 max-w-7xl items-center justify-between px-6">
				<Link href="/" className="flex items-center gap-3">
					<LogoMark />
					<span className="hidden text-lg font-bold tracking-tight text-white md:block">
						SpaceScale
					</span>
				</Link>
				<nav className="hidden items-center gap-8 md:flex">
					<Link
						className="text-sm font-medium text-railway-muted transition-colors hover:text-off-white"
						href="/"
					>
						Platform
					</Link>
					<Link
						className="text-sm font-medium text-railway-muted transition-colors hover:text-off-white"
						href="/"
					>
						Solutions
					</Link>
					<Link
						className="text-sm font-medium text-railway-muted transition-colors hover:text-off-white"
						href="/pricing"
					>
						Pricing
					</Link>
					<Link
						className="text-sm font-medium text-railway-muted transition-colors hover:text-off-white"
						href="/terms"
					>
						Docs
					</Link>
				</nav>
				<div className="flex items-center gap-4">
					<a
						className="text-sm font-medium text-railway-muted transition-colors hover:text-white"
						href={loginHref}
					>
						Login
					</a>
					<a
						className="group relative overflow-hidden rounded-full border hairline-border border-white/20 bg-white/[0.03] px-5 py-2 transition-all duration-300 hover:border-white/40 hover:bg-white/[0.08]"
						href={deployHref}
					>
						<span className="relative z-10 text-sm font-medium tracking-wide text-off-white transition-colors group-hover:text-white">
							Deploy Now
						</span>
					</a>
				</div>
			</div>
		</header>
	);
}
