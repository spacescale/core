import { MarketingHeader } from "@/components/marketing-header";
import { MarketingFooter } from "@/components/marketing-footer";
import { DashboardDemo } from "@/components/dashboard-demo";
import { getDashboardUrls } from "@/lib/urls";
import { CodeBlock, Badge, GlassCard, Separator } from "@spacescale/ui";

/* -------------------------------------------------------------------------- */
/*  Data                                                                      */
/* -------------------------------------------------------------------------- */

const steps = [
	{
		number: "01",
		title: "Push Code",
		description:
			"Connect your GitHub repository or push via CLI. SpaceScale detects changes instantly.",
		icon: "cloud_upload",
	},
	{
		number: "02",
		title: "Auto-Detect & Build",
		description:
			"We analyze your stack, install dependencies, and build optimized containers automatically.",
		icon: "auto_fix_high",
	},
	{
		number: "03",
		title: "Go Live",
		description:
			"Your service deploys globally in seconds with SSL, custom domains, and auto-scaling.",
		icon: "rocket_launch",
	},
];

const deployTerminalCode = `$ spacescale deploy

   Detected: Next.js 15 application
   Framework: next
   Build command: npm run build

   Building...
   ✓ Dependencies installed (4.2s)
   ✓ Application built (8.1s)
   ✓ Container optimized (1.3s)

   Deploying to production...
   ✓ Health check passed
   ✓ SSL certificate provisioned
   ✓ DNS propagated

   Live at: https://my-app.spacescale.app
   Deployed in 14.8s`;

const features = [
	{
		icon: "all_inclusive",
		title: "Instant Scale",
		description:
			"Horizontal scaling that adapts to traffic patterns in real-time. No manual provisioning required.",
	},
	{
		icon: "security",
		title: "Secure by Design",
		description:
			"Enterprise-grade encryption at rest and in transit. Automated security patching and compliance.",
	},
	{
		icon: "history",
		title: "Instant Rollbacks",
		description:
			"Traverse deployment history with single-second granularity. Undo any change instantly.",
	},
	{
		icon: "public",
		title: "Global Edge Network",
		description:
			"Content delivered from 300+ edge locations worldwide. Sub-50ms response times for every user.",
	},
	{
		icon: "monitoring",
		title: "Built-in Observability",
		description:
			"Real-time metrics, structured logs, and distributed tracing out of the box. No third-party setup.",
	},
	{
		icon: "group",
		title: "Team Collaboration",
		description:
			"Role-based access control, shared environments, deploy previews, and audit logs for your entire team.",
	},
];

const stats = [
	{ value: "5M+", label: "Live Services" },
	{ value: "100B+", label: "Monthly Requests" },
	{ value: "100M+", label: "Deploys" },
];

const testimonials = [
	{
		quote:
			"SpaceScale cut our deployment time from 45 minutes to under 30 seconds. Our engineering team ships features daily now instead of weekly.",
		name: "Sarah Chen",
		title: "VP of Engineering",
		company: "StreamVault",
	},
	{
		quote:
			"The observability tools alone are worth it. We caught a memory leak in staging before it ever hit production. The platform practically runs itself.",
		name: "Marcus Rivera",
		title: "CTO",
		company: "NeuralPath AI",
	},
	{
		quote:
			"We migrated 200 microservices in a weekend. The auto-detection of frameworks and zero-config deploys made what seemed impossible, effortless.",
		name: "Aisha Patel",
		title: "Head of Platform",
		company: "FinScale",
	},
];

const techStack = [
	{ name: "Next.js", icon: "web" },
	{ name: "React", icon: "code" },
	{ name: "Node.js", icon: "terminal" },
	{ name: "Python", icon: "data_object" },
	{ name: "Go", icon: "speed" },
	{ name: "Rust", icon: "memory" },
	{ name: "Docker", icon: "deployed_code" },
	{ name: "PostgreSQL", icon: "database" },
];

/* -------------------------------------------------------------------------- */
/*  Page                                                                      */
/* -------------------------------------------------------------------------- */

export default function MarketingHomePage() {
	const { deploy: deployHref } = getDashboardUrls();

	return (
		<div className="relative flex min-h-screen flex-col bg-railway-bg text-railway-text font-display antialiased overflow-x-hidden">
			<div className="fixed inset-0 bg-square-grid bg-grid-pattern pointer-events-none z-0 opacity-[0.8] grid-mask" />

			<MarketingHeader />

			<main className="relative z-10 flex flex-1 flex-col items-center pt-32 pb-20 px-6">
				{/* ── Hero ──────────────────────────────────────────────── */}
				<div className="max-w-4xl w-full text-center mx-auto mb-16 space-y-6">
					<h1 className="text-5xl md:text-7xl font-[200] tracking-tight text-off-white leading-[1.1]">
						A Smarter <br /> Deployment Platform.
					</h1>
					<p className="text-xl md:text-2xl text-railway-muted font-extralight tracking-wide leading-normal max-w-3xl mx-auto">
						Deploy Workloads with Maximum Scale, Simplicity, and
						Security.
					</p>
				</div>

				{/* ── Dashboard Demo ────────────────────────────────────── */}
				<DashboardDemo />

				{/* ── How It Works ──────────────────────────────────────── */}
				<section className="max-w-5xl mx-auto w-full mb-32 px-6">
					<div className="text-center mb-16">
						<h2 className="text-3xl md:text-5xl font-[200] text-off-white tracking-tight mb-4">
							How It Works
						</h2>
						<p className="text-lg text-railway-muted font-light max-w-2xl mx-auto">
							From code push to production in three seamless
							steps.
						</p>
					</div>

					<div className="relative grid grid-cols-1 md:grid-cols-3 gap-12 md:gap-8">
						{/* Connecting gradient line (desktop) */}
						<div className="hidden md:block absolute top-12 left-[16.67%] right-[16.67%] h-px bg-gradient-to-r from-indigo-500/0 via-indigo-500/40 to-indigo-500/0" />

						{steps.map((step) => (
							<div
								key={step.number}
								className="relative flex flex-col items-center text-center"
							>
								<div className="relative z-10 mb-6">
									<div className="w-24 h-24 rounded-full border border-white/10 bg-railway-card/50 backdrop-blur-sm flex items-center justify-center group hover:border-indigo-500/40 transition-all duration-500">
										<span className="material-symbols-outlined text-3xl text-indigo-400">
											{step.icon}
										</span>
									</div>
									<div className="absolute -top-2 -right-2 w-8 h-8 rounded-full bg-indigo-500/10 border border-indigo-500/30 flex items-center justify-center">
										<span className="text-xs font-mono font-medium text-indigo-300">
											{step.number}
										</span>
									</div>
								</div>
								<h3 className="text-lg font-medium text-off-white mb-2">
									{step.title}
								</h3>
								<p className="text-sm text-railway-muted font-light leading-relaxed max-w-xs">
									{step.description}
								</p>
							</div>
						))}
					</div>
				</section>

				{/* ── Logo Strip ────────────────────────────────────────── */}
				<div className="max-w-7xl mx-auto w-full mb-32 relative z-10 px-6">
					<div className="text-center mb-12">
						<h2 className="text-xl md:text-2xl font-[200] text-white/90 mb-10 tracking-tight">
							Trusted by over 4 million ambitious product builders
							and teams
						</h2>
						<div className="group relative flex gap-16 overflow-hidden logo-strip-mask opacity-40 grayscale hover:opacity-100 hover:grayscale-0 transition-all duration-700">
							{[0, 1].map((i) => (
								<div
									key={i}
									aria-hidden={i === 1}
									className="flex min-w-full shrink-0 items-center justify-around gap-16 animate-infinite-scroll"
								>
									<div className="flex items-center gap-2">
										<div className="w-6 h-6 rounded-full border-2 border-white flex items-center justify-center">
											<div className="w-2 h-2 bg-white rounded-full" />
										</div>
										<span className="text-xl font-bold text-white tracking-tight">
											twilio
										</span>
									</div>
									<div className="flex items-center gap-2">
										<span className="text-xl font-bold italic text-white">
											Alibaba.com
										</span>
									</div>
									<div className="flex items-center gap-2">
										<div className="w-5 h-5 rounded-full border border-white/60 border-dashed" />
										<span className="text-lg font-semibold text-white">
											Bridge
										</span>
									</div>
									<div className="flex items-center gap-2">
										<div className="w-6 h-6 rounded-full bg-white/20 relative overflow-hidden">
											<div className="absolute bottom-0 w-full h-1/2 bg-white" />
										</div>
										<span className="text-lg font-bold text-white">
											Base 44
										</span>
									</div>
									<div className="flex flex-col leading-none">
										<span className="text-xs font-bold text-white tracking-widest">
											CONTENT
										</span>
										<span className="text-xs font-bold text-white tracking-widest">
											SQUARE
										</span>
									</div>
								</div>
							))}
						</div>
					</div>
				</div>

				{/* ── Developer Experience ──────────────────────────────── */}
				<section className="max-w-7xl mx-auto w-full mb-32 px-6">
					<div className="grid grid-cols-1 md:grid-cols-2 gap-12 md:gap-16 items-center">
						<div className="space-y-6">
							<Badge
								variant="outline"
								className="border-indigo-500/30 text-indigo-300"
							>
								Developer Experience
							</Badge>
							<h2 className="text-3xl md:text-5xl font-[200] text-off-white tracking-tight leading-tight">
								Deploy in seconds,{" "}
								<span className="text-transparent bg-clip-text bg-gradient-to-r from-indigo-400 to-purple-400">
									not hours
								</span>
							</h2>
							<p className="text-lg text-railway-muted font-light leading-relaxed">
								A single command deploys your entire application.
								SpaceScale auto-detects your framework, builds
								optimized containers, provisions SSL, and routes
								traffic globally — all without configuration
								files.
							</p>
							<div className="flex items-center gap-4 pt-2">
								<a
									href={deployHref}
									className="group relative overflow-hidden rounded-full bg-indigo-600 px-6 py-2.5 text-sm font-medium text-white transition-all hover:bg-indigo-500 hover:shadow-[0_0_30px_-5px_rgba(99,102,241,0.5)]"
								>
									Get Started Free
								</a>
								<a
									href="/terms"
									className="text-sm font-medium text-railway-muted hover:text-off-white transition-colors"
								>
									Read the docs &rarr;
								</a>
							</div>
						</div>
						<div className="relative">
							<div className="absolute -inset-4 bg-indigo-500/5 blur-3xl rounded-3xl pointer-events-none" />
							<CodeBlock
								code={deployTerminalCode}
								title="Terminal"
								language="bash"
								showLineNumbers={false}
								className="relative z-10 border-white/10"
							/>
						</div>
					</div>
				</section>

				{/* ── Features ──────────────────────────────────────────── */}
				<section className="max-w-7xl mx-auto w-full mb-32 px-6">
					<div className="text-center mb-16">
						<h2 className="text-3xl md:text-5xl font-[200] text-off-white tracking-tight mb-4">
							Everything you need to ship
						</h2>
						<p className="text-lg text-railway-muted font-light max-w-2xl mx-auto">
							A complete platform for modern application
							deployment and operations.
						</p>
					</div>
					<div className="grid grid-cols-1 md:grid-cols-2 lg:grid-cols-3 gap-6">
						{features.map((f) => (
							<div
								key={f.title}
								className="p-6 rounded-xl border border-white/10 hover:border-white/20 bg-gradient-to-br from-white/[0.03] to-transparent transition-all duration-300 shadow-glass group hover:shadow-[0_8px_32px_rgba(99,102,241,0.08)]"
							>
								<div className="size-10 rounded-lg bg-indigo-500/10 border border-indigo-500/20 flex items-center justify-center mb-4 text-indigo-400 group-hover:bg-indigo-500/20 transition-colors">
									<span className="material-symbols-outlined font-light">
										{f.icon}
									</span>
								</div>
								<h3 className="text-lg font-medium text-off-white mb-2">
									{f.title}
								</h3>
								<p className="text-sm text-railway-muted leading-relaxed font-light">
									{f.description}
								</p>
							</div>
						))}
					</div>
				</section>

				{/* ── Stats ─────────────────────────────────────────────── */}
				<div className="max-w-7xl mx-auto w-full mb-32 relative z-10 px-6">
					<div className="grid grid-cols-1 md:grid-cols-3 gap-6">
						{stats.map((s) => (
							<div
								key={s.label}
								className="stats-glass-card rounded-xl p-8 flex flex-col justify-center items-start group hover:border-indigo-500/30 transition-all duration-300"
							>
								<h3 className="text-5xl md:text-6xl font-[200] text-white tracking-tighter mb-2 group-hover:text-glow-blue transition-all duration-500">
									{s.value}
								</h3>
								<p className="text-sm md:text-base font-light text-railway-muted tracking-wide">
									{s.label}
								</p>
							</div>
						))}
					</div>
				</div>

				{/* ── Testimonials ──────────────────────────────────────── */}
				<section className="max-w-7xl mx-auto w-full mb-32 px-6">
					<div className="text-center mb-16">
						<h2 className="text-3xl md:text-5xl font-[200] text-off-white tracking-tight mb-4">
							Loved by engineering teams
						</h2>
						<p className="text-lg text-railway-muted font-light max-w-2xl mx-auto">
							See why thousands of teams trust SpaceScale for
							production workloads.
						</p>
					</div>
					<div className="grid grid-cols-1 md:grid-cols-3 gap-6">
						{testimonials.map((t) => (
							<GlassCard
								key={t.name}
								variant="default"
								padding="lg"
							>
								<div className="flex flex-col h-full">
									<span className="text-3xl text-indigo-500/30 font-serif leading-none mb-4">
										&ldquo;
									</span>
									<p className="text-sm text-off-white/80 font-light leading-relaxed flex-1 mb-6">
										{t.quote}
									</p>
									<Separator className="bg-white/5 mb-4" />
									<div>
										<p className="text-sm font-medium text-off-white">
											{t.name}
										</p>
										<p className="text-xs text-railway-muted">
											{t.title}, {t.company}
										</p>
									</div>
								</div>
							</GlassCard>
						))}
					</div>
				</section>

				{/* ── Technology Support ────────────────────────────────── */}
				<section className="max-w-5xl mx-auto w-full mb-32 px-6">
					<div className="text-center mb-16">
						<h2 className="text-3xl md:text-5xl font-[200] text-off-white tracking-tight mb-4">
							Works with your stack
						</h2>
						<p className="text-lg text-railway-muted font-light max-w-2xl mx-auto">
							First-class support for every major language,
							framework, and runtime.
						</p>
					</div>
					<div className="grid grid-cols-2 md:grid-cols-4 gap-4">
						{techStack.map((tech) => (
							<div
								key={tech.name}
								className="flex items-center gap-3 rounded-xl border border-white/5 bg-white/[0.02] px-5 py-4 hover:border-white/15 hover:bg-white/[0.04] transition-all duration-300 group"
							>
								<span className="material-symbols-outlined text-xl text-railway-muted group-hover:text-indigo-400 transition-colors">
									{tech.icon}
								</span>
								<span className="text-sm font-medium text-off-white/80 group-hover:text-off-white transition-colors">
									{tech.name}
								</span>
							</div>
						))}
					</div>
				</section>

				{/* ── CTA ───────────────────────────────────────────────── */}
				<section className="max-w-4xl mx-auto w-full mb-20 px-6 relative">
					<div className="absolute inset-0 -z-10">
						<div className="absolute top-1/2 left-1/2 -translate-x-1/2 -translate-y-1/2 w-[500px] h-[300px] bg-indigo-500/15 blur-[120px] rounded-full pointer-events-none" />
					</div>

					<div className="text-center space-y-6 py-20">
						<h2 className="text-4xl md:text-6xl font-[200] tracking-tight leading-tight">
							<span className="text-transparent bg-clip-text bg-gradient-to-r from-indigo-400 via-purple-400 to-indigo-400">
								Ready to ship?
							</span>
						</h2>
						<p className="text-lg md:text-xl text-railway-muted font-light max-w-xl mx-auto leading-relaxed">
							Join over 4 million developers deploying with
							SpaceScale. Start free, scale without limits.
						</p>
						<div className="flex items-center justify-center gap-4 pt-4">
							<a
								href={deployHref}
								className="group relative overflow-hidden rounded-full bg-indigo-600 px-8 py-3 text-sm font-medium text-white transition-all hover:bg-indigo-500 hover:shadow-[0_0_40px_-5px_rgba(99,102,241,0.5)]"
							>
								<span className="absolute inset-0 bg-gradient-to-r from-transparent via-white/10 to-transparent -translate-x-full group-hover:animate-shimmer" />
								<span className="relative z-10">
									Get Started Free
								</span>
							</a>
						</div>
					</div>
				</section>
			</main>

			<MarketingFooter />
		</div>
	);
}
