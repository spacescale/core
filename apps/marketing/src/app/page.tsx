import { LogoMark } from "@spacescale/ui";
import Link from "next/link";
import { useId } from "react";

const DEFAULT_DASHBOARD_URL = "http://localhost:3000";

function sanitizeBaseUrl(url: string): string {
	return url.replace(/\/+$/, "");
}

export default function MarketingHomePage() {
	const dashboardBaseUrl = sanitizeBaseUrl(
		process.env.NEXT_PUBLIC_DASHBOARD_URL ?? DEFAULT_DASHBOARD_URL,
	);
	const loginHref = `${dashboardBaseUrl}/login`;
	const deployHref = loginHref;
	const currentYear = new Date().getFullYear();
	const waveGradientId = useId().replace(/:/g, "");

	return (
		<div className="relative flex min-h-screen flex-col bg-railway-bg text-railway-text font-display antialiased overflow-x-hidden">
			<div className="fixed inset-0 bg-square-grid bg-grid-pattern pointer-events-none z-0 opacity-[0.8] grid-mask"></div>
			<header className="fixed top-0 z-50 w-full backdrop-blur-md border-b hairline-border border-railway-border/40 bg-railway-bg/70">
				<div className="mx-auto flex h-16 max-w-7xl items-center justify-between px-6">
					<div className="flex items-center gap-3">
						<LogoMark />
						<span className="hidden text-lg font-bold tracking-tight text-white md:block">
							SpaceScale
						</span>
					</div>
					<nav className="hidden md:flex items-center gap-8">
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
			<main className="relative z-10 flex flex-1 flex-col items-center pt-32 pb-20 px-6">
				<div className="max-w-4xl w-full text-center mx-auto mb-16 space-y-6">
					<h1 className="text-5xl md:text-7xl font-[200] tracking-tight text-off-white leading-[1.1]">
						A Smarter <br /> Deployment Platform.
					</h1>
					<p className="text-xl md:text-2xl text-railway-muted font-extralight tracking-wide leading-normal max-w-3xl mx-auto">
						Deploy Workloads with Maximum Scale, Simplicity, and Security.
					</p>
				</div>
				<div className="w-full max-w-6xl mx-auto mb-16 relative">
					<div className="absolute -top-20 -left-20 w-[400px] h-[400px] bg-indigo-500/10 blur-[100px] rounded-full pointer-events-none"></div>
					<div className="absolute -bottom-20 -right-20 w-[400px] h-[400px] bg-emerald-500/5 blur-[100px] rounded-full pointer-events-none"></div>
					<div className="glass-panel rounded-xl overflow-hidden shadow-2xl relative z-10 flex h-[600px] w-full border border-white/10">
						<div className="w-64 border-r border-white/5 bg-[#0b0d14]/50 flex flex-col hidden md:flex shrink-0">
							<div className="p-4 border-b border-white/5 flex items-center justify-between bg-white/[0.02]">
								<div className="flex items-center gap-3">
									<div className="w-8 h-8 rounded bg-indigo-500/10 border border-indigo-500/20 text-indigo-400 flex items-center justify-center text-[10px] font-bold font-mono">
										A
									</div>
									<div className="flex flex-col">
										<span className="text-xs font-medium text-white tracking-wide">
											SpaceScale
										</span>
										<span className="text-[9px] text-railway-muted uppercase tracking-wider font-mono">
											Personal
										</span>
									</div>
								</div>
							</div>
							<div className="flex-1 overflow-y-auto py-6 px-3 space-y-1 custom-scrollbar">
								<div className="flex items-center justify-between px-3 py-2 rounded text-sm text-white bg-white/[0.05] border-l-2 border-indigo-500">
									<div className="flex items-center gap-3">
										<span className="material-symbols-outlined text-[18px] text-indigo-400">
											layers
										</span>
										<span>Applications</span>
									</div>
								</div>
								<div className="flex items-center justify-between px-3 py-2 rounded text-sm text-railway-muted">
									<div className="flex items-center gap-3">
										<span className="material-symbols-outlined text-[18px]">
											memory
										</span>
										<span>Workers</span>
									</div>
								</div>
								<div className="flex items-center justify-between px-3 py-2 rounded text-sm text-railway-muted">
									<div className="flex items-center gap-3">
										<span className="material-symbols-outlined text-[18px]">
											functions
										</span>
										<span>Functions</span>
									</div>
								</div>
								<div className="flex items-center justify-between px-3 py-2 rounded text-sm text-railway-muted">
									<div className="flex items-center gap-3">
										<span className="material-symbols-outlined text-[18px]">
											database
										</span>
										<span>Databases</span>
									</div>
								</div>
							</div>
							<div className="mt-auto p-4 border-t border-white/5 bg-white/[0.01]">
								<div className="flex items-center gap-3">
									<div className="w-7 h-7 rounded-full bg-gradient-to-tr from-gray-700 to-gray-600 flex items-center justify-center text-[10px] font-bold text-white">
										AS
									</div>
									<div className="flex flex-col">
										<span className="text-xs text-white">Adam Smith</span>
										<span className="text-[10px] text-white/40">Pro Plan</span>
									</div>
								</div>
							</div>
						</div>
						<div className="flex-1 relative bg-[#0f111a]/30 backdrop-blur-xl overflow-hidden">
							<div className="animate-cursor-main absolute z-[100] pointer-events-none drop-shadow-2xl opacity-0">
								<svg
									fill="none"
									height="24"
									viewBox="0 0 24 24"
									width="24"
									xmlns="http://www.w3.org/2000/svg"
								>
									<title>Animated cursor pointer</title>
									<path
										d="M5.65376 12.3673H5.46026L5.31717 12.4976L0.500002 16.8829L0.500002 1.19138L11.7841 12.3673H5.65376Z"
										fill="white"
										stroke="black"
										strokeWidth="1"
									></path>
								</svg>
							</div>
							<div className="absolute inset-0 flex flex-col animate-view-1">
								<div className="h-16 border-b border-white/5 flex items-center justify-between px-8 bg-white/[0.02]">
									<h2 className="text-sm font-medium text-white">
										Applications
									</h2>
									<button
										type="button"
										className="group relative px-4 py-1.5 rounded-full bg-white/[0.05] border border-white/10 overflow-hidden hover:bg-white/[0.08] hover:border-white/20 transition-all shadow-[0_0_20px_-5px_rgba(255,255,255,0.1)] backdrop-blur-md"
									>
										<span className="relative z-10 text-xs font-medium text-white tracking-wide">
											Deploy New
										</span>
									</button>
								</div>
								<div className="p-8">
									<div className="grid grid-cols-1 gap-4">
										<div className="bg-railway-card border border-white/5 rounded-lg p-4 flex items-center justify-between group hover:border-white/10 transition-all">
											<div className="flex items-center gap-4">
												<div className="w-10 h-10 rounded-full bg-emerald-500/10 border border-emerald-500/20 flex items-center justify-center text-emerald-400">
													<span className="material-symbols-outlined text-lg">
														shield
													</span>
												</div>
												<div>
													<div className="text-sm font-medium text-white">
														auth-service
													</div>
													<div className="text-[10px] text-railway-muted font-mono">
														Running &bull; 2 instances
													</div>
												</div>
											</div>
											<div className="flex items-center gap-2 px-2 py-1 bg-emerald-500/10 rounded border border-emerald-500/20">
												<div className="w-1.5 h-1.5 rounded-full bg-emerald-400 animate-pulse"></div>
												<span className="text-[10px] font-medium text-emerald-400 uppercase tracking-wide">
													Healthy
												</span>
											</div>
										</div>
									</div>
								</div>
							</div>
							<div className="absolute inset-0 flex flex-col items-center justify-center animate-view-2 opacity-0">
								<div className="w-full max-w-2xl px-8 relative h-full flex flex-col justify-center">
									<div className="animate-view-2-content absolute inset-x-0 top-1/2 -translate-y-1/2 px-8">
										<h2 className="text-xl font-light text-white text-center mb-8">
											New Resource
										</h2>
										<div className="grid grid-cols-2 gap-6">
											<div className="bg-railway-card/50 border border-white/10 rounded-xl p-6 hover:bg-railway-card transition-all cursor-pointer group hover:border-indigo-500/50 relative overflow-hidden">
												<div className="absolute inset-0 bg-gradient-to-br from-indigo-500/10 to-transparent opacity-0 group-hover:opacity-100 transition-opacity"></div>
												<span className="material-symbols-outlined text-3xl text-white mb-4">
													code
												</span>
												<h3 className="text-sm font-medium text-white mb-1">
													GitHub Repository
												</h3>
												<p className="text-xs text-railway-muted">
													Connect a repository to auto-deploy.
												</p>
											</div>
											<div className="bg-railway-card/50 border border-white/10 rounded-xl p-6 hover:bg-railway-card transition-all opacity-50">
												<span className="material-symbols-outlined text-3xl text-white mb-4">
													box
												</span>
												<h3 className="text-sm font-medium text-white mb-1">
													Container Registry
												</h3>
												<p className="text-xs text-railway-muted">
													Deploy an existing image.
												</p>
											</div>
										</div>
									</div>
									<div className="absolute inset-0 z-20 flex flex-col items-center justify-center animate-view-2-scan opacity-0 hidden pointer-events-none">
										<div className="flex flex-col items-center gap-4">
											<div className="relative size-12">
												<div className="absolute inset-0 rounded-full border-2 border-white/10"></div>
												<div className="absolute inset-0 rounded-full border-t-2 border-indigo-500 animate-spin"></div>
											</div>
											<span className="text-xs font-mono text-indigo-300">
												Analyzing repository...
											</span>
										</div>
									</div>
									<div className="absolute inset-x-0 top-1/2 -translate-y-1/2 px-8 flex flex-col items-center animate-view-2-result opacity-0 hidden">
										<div className="text-center mb-12 w-full relative z-10">
											<h2 className="text-3xl md:text-4xl font-[200] text-white tracking-tight drop-shadow-md">
												We found 2 deployable services
											</h2>
										</div>
										<div className="grid grid-cols-2 gap-6 w-full max-w-2xl relative z-10">
											<div className="h-48 rounded-xl border border-white/10 bg-white/[0.02] hover:bg-white/[0.04] hover:border-white/20 transition-all duration-300 cursor-pointer flex flex-col items-center justify-center gap-4 group backdrop-blur-sm">
												<span className="material-symbols-outlined text-white/30 text-4xl font-light group-hover:text-white/60 transition-colors">
													web
												</span>
												<span className="text-xs font-light text-white/40 tracking-widest uppercase group-hover:text-white/80 transition-colors">
													Static Site
												</span>
											</div>
											<div className="h-48 rounded-xl border border-indigo-500/40 bg-indigo-500/10 transition-all duration-300 cursor-pointer flex flex-col items-center justify-center gap-4 shadow-[0_0_50px_-10px_rgba(99,102,241,0.4)] ring-1 ring-indigo-500/30 backdrop-blur-sm relative overflow-hidden">
												<div className="absolute top-4 right-4 w-1.5 h-1.5 rounded-full bg-indigo-400 shadow-[0_0_8px_rgba(99,102,241,0.8)] animate-pulse"></div>
												<span className="material-symbols-outlined text-indigo-300 text-4xl font-light">
													api
												</span>
												<span className="text-xs font-medium text-white tracking-widest uppercase text-shadow-sm">
													Web Service
												</span>
											</div>
										</div>
									</div>
								</div>
							</div>
							<div className="absolute inset-0 flex flex-col animate-view-3 opacity-0">
								<div className="flex flex-col border-b border-white/5 bg-[#0f111a]/90 backdrop-blur-sm z-30">
									<div className="h-14 flex items-center justify-between px-6">
										<div className="flex items-center gap-4">
											<div className="flex items-center text-sm font-medium">
												<span className="text-railway-muted hover:text-white cursor-pointer transition-colors">
													Applications
												</span>
												<span className="mx-2 text-white/10">/</span>
												<span className="flex items-center gap-2 text-white">
													<div className="w-2 h-2 rounded-full bg-emerald-500 shadow-[0_0_8px_rgba(16,185,129,0.4)]"></div>
													vibrant-nebula
												</span>
											</div>
										</div>
										<div className="flex items-center gap-3">
											<div className="flex items-center gap-2 px-3 py-1.5 rounded bg-black/40 border border-white/10 shadow-sm">
												<span className="material-symbols-outlined text-white/40 text-xs">
													link
												</span>
												<span className="text-xs font-mono text-emerald-400 transition-colors">
													vibrant-nebula.alpha.spacescale.ai
												</span>
											</div>
											<button
												type="button"
												className="relative group px-4 py-1.5 text-xs font-medium bg-white/[0.05] border border-white/10 rounded-full hover:bg-white/[0.08] hover:border-white/20 hover:shadow-[0_0_15px_rgba(255,255,255,0.05)] transition-all flex items-center gap-1.5 overflow-hidden"
											>
												<span className="absolute inset-0 bg-gradient-to-r from-transparent via-white/10 to-transparent -translate-x-full group-hover:animate-shimmer"></span>
												<span className="text-off-white tracking-wide z-10">
													Visit
												</span>
												<span className="material-symbols-outlined text-[14px] text-off-white z-10 group-hover:translate-x-0.5 transition-transform">
													arrow_forward
												</span>
											</button>
										</div>
									</div>
									<div className="flex items-center px-4 relative border-t border-white/5">
										<button
											type="button"
											className="px-4 py-2.5 text-xs font-medium text-railway-muted hover:text-white transition-colors relative z-10"
										>
											Overview
										</button>
										<button
											type="button"
											className="px-4 py-2.5 text-xs font-medium text-railway-muted hover:text-white transition-colors relative z-10"
										>
											Metrics
										</button>
										<button
											type="button"
											className="px-4 py-2.5 text-xs font-medium text-railway-muted hover:text-white transition-colors relative z-10"
										>
											Logs
										</button>
										<button
											type="button"
											className="px-4 py-2.5 text-xs font-medium text-railway-muted hover:text-white transition-colors relative z-10"
										>
											Settings
										</button>
										<div className="absolute bottom-0 h-[2px] bg-indigo-500 animate-tab-highlight"></div>
									</div>
								</div>
								<div className="flex-1 relative bg-slate-900/20 overflow-hidden">
									<div className="absolute inset-0 p-8 overflow-y-auto custom-scrollbar animate-dash-build">
										<div className="flex items-center justify-between mb-6">
											<div className="flex items-center gap-3">
												<div className="w-8 h-8 flex items-center justify-center rounded bg-indigo-500/10 border border-indigo-500/20 text-indigo-400">
													<span className="material-symbols-outlined text-sm animate-spin">
														sync
													</span>
												</div>
												<div>
													<h3 className="text-sm font-medium text-white">
														Deploying...
													</h3>
													<p className="text-[10px] text-railway-muted font-mono">
														Build ID: bld-8x92a
													</p>
												</div>
											</div>
											<span className="text-xs font-mono text-white/40">
												00:04
											</span>
										</div>
										<div className="h-0.5 w-full bg-white/5 rounded-full mb-6 overflow-hidden">
											<div className="h-full bg-gradient-to-r from-indigo-500 to-purple-500 animate-progress-fill rounded-full shadow-[0_0_15px_rgba(99,102,241,0.6)]"></div>
										</div>
										<div className="h-64 bg-[#0a0a0a] rounded border border-white/5 p-4 overflow-hidden relative font-mono text-[10px] leading-[1.6]">
											<div className="animate-term-scroll">
												<div className="flex gap-3 text-white/40">
													<span className="w-12 text-white/20">10:42:01</span>
													<span className="text-indigo-400">info</span>{" "}
													<span>Initializing build environment...</span>
												</div>
												<div className="flex gap-3 text-white/40">
													<span className="w-12 text-white/20">10:42:02</span>
													<span className="text-indigo-400">info</span>{" "}
													<span>Pulling repository metadata</span>
												</div>
												<div className="flex gap-3 text-white/40">
													<span className="w-12 text-white/20">10:42:05</span>
													<span className="text-yellow-500">warn</span>{" "}
													<span>Cache miss for node_modules</span>
												</div>
												<div className="flex gap-3 text-white/40">
													<span className="w-12 text-white/20">10:42:08</span>
													<span className="text-indigo-400">info</span>{" "}
													<span>Installing dependencies...</span>
												</div>
												<div className="flex gap-3 text-white/40">
													<span className="w-12 text-white/20">10:42:15</span>
													<span className="text-indigo-400">info</span>{" "}
													<span>Building static pages</span>
												</div>
												<div className="flex gap-3 text-white/40">
													<span className="w-12 text-white/20">10:42:18</span>
													<span className="text-indigo-400">info</span>{" "}
													<span>Optimizing assets</span>
												</div>
												<div className="flex gap-3 text-white/40">
													<span className="w-12 text-white/20">10:42:22</span>
													<span className="text-indigo-400">info</span>{" "}
													<span>Uploading build artifacts</span>
												</div>
												<div className="flex gap-3 text-white/40">
													<span className="w-12 text-white/20">10:42:26</span>
													<span className="text-emerald-500">succ</span>{" "}
													<span>Deployment active</span>
												</div>
											</div>
										</div>
									</div>
									<div className="absolute inset-0 p-8 overflow-y-auto custom-scrollbar animate-dash-metrics opacity-0">
										<div className="grid grid-cols-2 gap-6 h-full">
											<div className="bg-[#12141c]/80 border hairline-border border-white/5 rounded-xl p-6 relative flex flex-col shadow-lg overflow-hidden">
												<div className="absolute inset-0 bg-gradient-to-br from-indigo-500/5 to-transparent pointer-events-none"></div>
												<div className="flex justify-between items-start mb-6 z-10">
													<div>
														<h3 className="text-xs font-semibold text-white/90 tracking-wide uppercase">
															CPU Load
														</h3>
														<p className="text-[10px] text-railway-muted mt-1">
															Real-time processing
														</p>
													</div>
													<div className="flex flex-col items-end">
														<span className="text-xl font-light text-white font-mono tracking-tight">
															0.42%
														</span>
														<span className="text-[9px] text-indigo-400 font-mono">
															peak: 1.2%
														</span>
													</div>
												</div>
												<div className="flex-1 flex items-end justify-center relative z-10 w-full overflow-hidden">
													<svg
														className="w-full h-32"
														preserveAspectRatio="none"
														viewBox="0 0 100 50"
													>
														<title>CPU load waveform chart</title>
														<defs>
															<linearGradient
																id={waveGradientId}
																x1="0"
																x2="0"
																y1="0"
																y2="1"
															>
																<stop
																	offset="0%"
																	stopColor="#6366f1"
																	stopOpacity="0.4"
																></stop>
																<stop
																	offset="100%"
																	stopColor="#6366f1"
																	stopOpacity="0"
																></stop>
															</linearGradient>
														</defs>
														<path
															className="animate-wave-flow"
															d="M0,50 C20,40 40,45 60,35 S80,20 100,25 S120,40 140,50 V50 H0 Z"
															fill={`url(#${waveGradientId})`}
														></path>
														<path
															className="animate-wave-flow"
															d="M0,50 C20,35 40,40 60,30 S80,15 100,20 S120,35 140,50 V50 H0 Z"
															fill="none"
															opacity="0.8"
															stroke="#818cf8"
															strokeWidth="0.5"
														></path>
													</svg>
												</div>
												<div className="flex justify-between text-[9px] text-white/20 mt-2 font-mono border-t border-white/5 pt-2">
													<span>Now</span>
													<span>-15s</span>
													<span>-30s</span>
												</div>
											</div>
											<div className="bg-[#12141c]/80 border hairline-border border-white/5 rounded-xl p-6 relative flex flex-col shadow-lg">
												<div className="flex justify-between items-start mb-6 z-10">
													<div>
														<h3 className="text-xs font-semibold text-white/90 tracking-wide uppercase">
															Memory
														</h3>
														<p className="text-[10px] text-railway-muted mt-1">
															Heap allocation
														</p>
													</div>
													<div className="flex flex-col items-end">
														<span className="text-xl font-light text-white font-mono tracking-tight">
															128MB
														</span>
														<span className="text-[9px] text-railway-muted font-mono">
															/ 512MB limit
														</span>
													</div>
												</div>
												<div className="relative flex-1 flex items-center justify-center">
													<div className="relative w-40 h-40 flex items-center justify-center">
														<div className="absolute inset-0 border border-white/5 rounded-full"></div>
														<div className="absolute inset-8 border border-white/5 rounded-full"></div>
														<div className="absolute inset-16 border border-white/5 rounded-full"></div>
														<div className="absolute inset-0 rounded-full border border-indigo-500/30 animate-radar-pulse"></div>
														<div className="absolute inset-0 rounded-full bg-indigo-500/5 blur-xl"></div>
														<svg className="absolute w-full h-full transform -rotate-90 drop-shadow-[0_0_8px_rgba(99,102,241,0.5)]">
															<title>Memory utilization radial indicator</title>
															<circle
																cx="80"
																cy="80"
																fill="none"
																r="30"
																stroke="#6366f1"
																strokeDasharray="188"
																strokeDashoffset="140"
																strokeLinecap="round"
																strokeWidth="1.5"
															></circle>
														</svg>
														<div className="absolute text-center">
															<span className="block text-2xl font-light text-white tracking-tighter">
																25%
															</span>
															<span className="text-[9px] text-indigo-300 uppercase tracking-widest font-semibold">
																Stable
															</span>
														</div>
													</div>
												</div>
											</div>
										</div>
									</div>
									<div className="absolute inset-0 p-0 overflow-hidden animate-dash-logs opacity-0 bg-[#0a0a0a]">
										<div className="h-10 border-b border-white/5 flex items-center px-4 gap-4 bg-[#121212]">
											<span className="material-symbols-outlined text-white/20 text-sm">
												search
											</span>
											<span className="text-xs text-white/30 font-mono">
												Filter logs...
											</span>
											<div className="ml-auto flex gap-2">
												<div className="px-2 py-0.5 rounded bg-white/5 text-[9px] text-white/40 border border-white/5">
													ALL
												</div>
											</div>
										</div>
										<div className="p-4 font-mono text-[11px] leading-relaxed overflow-y-hidden h-full">
											<div className="code-line">
												<span className="code-time">10:52:01</span>
												<span className="code-content text-blue-400">
													[INFO] Incoming request GET /api/v1/users
												</span>
											</div>
											<div className="code-line">
												<span className="code-time">10:52:01</span>
												<span className="code-content text-white/60">
													Processing in worker-node-ae2
												</span>
											</div>
											<div className="code-line">
												<span className="code-time">10:52:02</span>
												<span className="code-content text-green-400">
													[DB] Query executed (4ms)
												</span>
											</div>
											<div className="code-line">
												<span className="code-time">10:52:02</span>
												<span className="code-content text-blue-400">
													[INFO] Response sent 200 OK
												</span>
											</div>
											<div className="code-line">
												<span className="code-time">10:52:05</span>
												<span className="code-content text-blue-400">
													[INFO] Incoming request GET /api/v1/dashboard
												</span>
											</div>
											<div className="code-line">
												<span className="code-time">10:52:06</span>
												<span className="code-content text-yellow-400">
													[WARN] High latency detected in region: us-east
												</span>
											</div>
											<div className="code-line">
												<span className="code-time">10:52:07</span>
												<span className="code-content text-white/60">
													Scaling up instance count...
												</span>
											</div>
											<div className="code-line">
												<span className="code-time">10:52:08</span>
												<span className="code-content text-green-400">
													[SYS] New instance vibrant-nebula-x2 ready
												</span>
											</div>
											<div className="code-line">
												<span className="code-time">10:52:09</span>
												<span className="code-content text-blue-400">
													[INFO] Load balanced successfully
												</span>
											</div>
											<div className="code-line">
												<span className="code-time">10:52:12</span>
												<span className="code-content text-blue-400">
													[INFO] Incoming request POST /auth/session
												</span>
											</div>
											<div className="code-line">
												<span className="code-time">10:52:13</span>
												<span className="code-content text-purple-400">
													[AUTH] Token validated
												</span>
											</div>
										</div>
									</div>
									<div className="absolute inset-0 p-8 overflow-y-auto custom-scrollbar animate-dash-settings opacity-0">
										<h3 className="text-sm font-medium text-white mb-6">
											Environment Variables
										</h3>
										<div className="bg-[#12141c]/80 border border-white/5 rounded-lg overflow-hidden">
											<div className="grid grid-cols-12 gap-4 px-4 py-3 border-b border-white/5 bg-white/[0.02] text-[10px] uppercase font-semibold text-white/40">
												<div className="col-span-4">Key</div>
												<div className="col-span-6">Value</div>
												<div className="col-span-2 text-right">Action</div>
											</div>
											<div className="grid grid-cols-12 gap-4 px-4 py-3 border-b border-white/5 text-xs text-white items-center">
												<div className="col-span-4 font-mono text-indigo-300">
													DATABASE_URL
												</div>
												<div className="col-span-6 font-mono text-white/30">
													postgres://user:pass@...
												</div>
												<div className="col-span-2 text-right">
													<span className="material-symbols-outlined text-white/20 text-sm cursor-pointer hover:text-white">
														visibility
													</span>
												</div>
											</div>
											<div className="grid grid-cols-12 gap-4 px-4 py-3 border-b border-white/5 text-xs text-white items-center">
												<div className="col-span-4 font-mono text-indigo-300">
													API_SECRET_KEY
												</div>
												<div className="col-span-6 font-mono text-white/30">
													********************
												</div>
												<div className="col-span-2 text-right">
													<span className="material-symbols-outlined text-white/20 text-sm cursor-pointer hover:text-white">
														visibility
													</span>
												</div>
											</div>
											<div className="grid grid-cols-12 gap-4 px-4 py-3 text-xs text-white items-center">
												<div className="col-span-4 font-mono text-indigo-300">
													NODE_ENV
												</div>
												<div className="col-span-6 font-mono text-white">
													production
												</div>
												<div className="col-span-2 text-right">
													<span className="material-symbols-outlined text-white/20 text-sm cursor-pointer hover:text-white">
														edit
													</span>
												</div>
											</div>
										</div>
										<div className="mt-4 flex justify-end">
											<button
												type="button"
												className="px-3 py-1.5 bg-indigo-600 hover:bg-indigo-500 text-white text-xs rounded transition-colors"
											>
												Save Changes
											</button>
										</div>
									</div>
								</div>
							</div>
							<div className="absolute inset-0 flex flex-col animate-view-4 opacity-0 pointer-events-none">
								<div className="h-16 border-b border-white/5 flex items-center justify-between px-8 bg-white/[0.02]">
									<h2 className="text-sm font-medium text-white">
										Applications
									</h2>
									<button
										type="button"
										className="group relative px-4 py-1.5 rounded-full bg-white/[0.05] border border-white/10 overflow-hidden hover:bg-white/[0.08] hover:border-white/20 transition-all shadow-[0_0_20px_-5px_rgba(255,255,255,0.1)] backdrop-blur-md"
									>
										<span className="relative z-10 text-xs font-medium text-white tracking-wide">
											Deploy New
										</span>
									</button>
								</div>
								<div className="p-8">
									<div className="grid grid-cols-1 gap-4">
										<div className="bg-railway-card border border-white/5 rounded-lg p-4 flex items-center justify-between">
											<div className="flex items-center gap-4">
												<div className="w-10 h-10 rounded-full bg-emerald-500/10 border border-emerald-500/20 flex items-center justify-center text-emerald-400">
													<span className="material-symbols-outlined text-lg">
														shield
													</span>
												</div>
												<div>
													<div className="text-sm font-medium text-white">
														auth-service
													</div>
													<div className="text-[10px] text-railway-muted font-mono">
														Running &bull; 2 instances
													</div>
												</div>
											</div>
											<div className="flex items-center gap-2 px-2 py-1 bg-emerald-500/10 rounded border border-emerald-500/20">
												<span className="text-[10px] font-medium text-emerald-400 uppercase tracking-wide">
													Healthy
												</span>
											</div>
										</div>
										<div className="bg-railway-card border border-white/5 rounded-lg p-4 flex items-center justify-between animate-[pulse_2s_ease-out]">
											<div className="flex items-center gap-4">
												<div className="w-10 h-10 rounded-full bg-indigo-500/10 border border-indigo-500/20 flex items-center justify-center text-indigo-400">
													<span className="material-symbols-outlined text-lg">
														rocket_launch
													</span>
												</div>
												<div>
													<div className="text-sm font-medium text-white">
														vibrant-nebula
													</div>
													<div className="text-[10px] text-railway-muted font-mono">
														Just deployed &bull; 1 instance
													</div>
												</div>
											</div>
											<div className="flex items-center gap-2 px-2 py-1 bg-indigo-500/10 rounded border border-indigo-500/20">
												<div className="w-1.5 h-1.5 rounded-full bg-indigo-400 animate-pulse"></div>
												<span className="text-[10px] font-medium text-indigo-400 uppercase tracking-wide">
													Live
												</span>
											</div>
										</div>
									</div>
								</div>
							</div>
						</div>
					</div>
				</div>
				<div className="max-w-7xl mx-auto w-full mb-32 relative z-10 px-6">
					<div className="text-center mb-12">
						<h2 className="text-xl md:text-2xl font-[200] text-white/90 mb-10 tracking-tight">
							Trusted by over 4 million ambitious product builders and teams
						</h2>
						<div className="group relative flex gap-16 overflow-hidden logo-strip-mask opacity-40 grayscale hover:opacity-100 hover:grayscale-0 transition-all duration-700">
							<div className="flex min-w-full shrink-0 items-center justify-around gap-16 animate-infinite-scroll">
								<div className="flex items-center gap-2">
									<div className="w-6 h-6 rounded-full border-2 border-white flex items-center justify-center">
										<div className="w-2 h-2 bg-white rounded-full"></div>
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
									<div className="w-5 h-5 rounded-full border border-white/60 border-dashed"></div>
									<span className="text-lg font-semibold text-white">
										Bridge
									</span>
								</div>
								<div className="flex items-center gap-2">
									<div className="w-6 h-6 rounded-full bg-white/20 relative overflow-hidden">
										<div className="absolute bottom-0 w-full h-1/2 bg-white"></div>
									</div>
									<span className="text-lg font-bold text-white">Base 44</span>
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
							<div
								aria-hidden="true"
								className="flex min-w-full shrink-0 items-center justify-around gap-16 animate-infinite-scroll"
							>
								<div className="flex items-center gap-2">
									<div className="w-6 h-6 rounded-full border-2 border-white flex items-center justify-center">
										<div className="w-2 h-2 bg-white rounded-full"></div>
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
									<div className="w-5 h-5 rounded-full border border-white/60 border-dashed"></div>
									<span className="text-lg font-semibold text-white">
										Bridge
									</span>
								</div>
								<div className="flex items-center gap-2">
									<div className="w-6 h-6 rounded-full bg-white/20 relative overflow-hidden">
										<div className="absolute bottom-0 w-full h-1/2 bg-white"></div>
									</div>
									<span className="text-lg font-bold text-white">Base 44</span>
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
						</div>
					</div>
				</div>
				<div className="max-w-7xl mx-auto w-full grid grid-cols-1 md:grid-cols-3 gap-8 px-4 mb-48">
					<div className="p-6 rounded-lg border border-white/10 hover:border-white/20 bg-gradient-to-br from-white/[0.03] to-transparent transition-colors shadow-glass">
						<div className="size-10 rounded bg-[#1e2230] border border-white/5 flex items-center justify-center mb-4 text-off-white">
							<span className="material-symbols-outlined font-light">
								all_inclusive
							</span>
						</div>
						<h3 className="text-lg font-medium text-off-white mb-2">
							Instant Scale
						</h3>
						<p className="text-sm text-railway-muted leading-relaxed font-light">
							Horizontal scaling that adapts to traffic patterns in real-time.
							No manual provisioning required.
						</p>
					</div>
					<div className="p-6 rounded-lg border border-white/10 hover:border-white/20 bg-gradient-to-br from-white/[0.03] to-transparent transition-colors shadow-glass">
						<div className="size-10 rounded bg-[#1e2230] border border-white/5 flex items-center justify-center mb-4 text-off-white">
							<span className="material-symbols-outlined font-light">
								security
							</span>
						</div>
						<h3 className="text-lg font-medium text-off-white mb-2">
							Secure by Design
						</h3>
						<p className="text-sm text-railway-muted leading-relaxed font-light">
							Enterprise-grade encryption at rest and in transit. Automated
							security patching and compliance.
						</p>
					</div>
					<div className="p-6 rounded-lg border border-white/10 hover:border-white/20 bg-gradient-to-br from-white/[0.03] to-transparent transition-colors shadow-glass">
						<div className="size-10 rounded bg-[#1e2230] border border-white/5 flex items-center justify-center mb-4 text-off-white">
							<span className="material-symbols-outlined font-light">
								history
							</span>
						</div>
						<h3 className="text-lg font-medium text-off-white mb-2">
							Instant Rollbacks
						</h3>
						<p className="text-sm text-railway-muted leading-relaxed font-light">
							Traverse deployment history with single-second granularity. Undo
							any change instantly.
						</p>
					</div>
				</div>
				<div className="max-w-7xl mx-auto w-full mb-32 relative z-10 px-6">
					<div className="grid grid-cols-1 md:grid-cols-3 gap-6 pt-0 border-t-0">
						<div className="stats-glass-card rounded-xl p-8 flex flex-col justify-center items-start group hover:border-indigo-500/30 transition-all duration-300">
							<h3 className="text-5xl md:text-6xl font-[200] text-white tracking-tighter mb-2 group-hover:text-glow-blue transition-all duration-500">
								5M+
							</h3>
							<p className="text-sm md:text-base font-light text-railway-muted tracking-wide">
								Live Services
							</p>
						</div>
						<div className="stats-glass-card rounded-xl p-8 flex flex-col justify-center items-start group hover:border-indigo-500/30 transition-all duration-300">
							<h3 className="text-5xl md:text-6xl font-[200] text-white tracking-tighter mb-2 group-hover:text-glow-blue transition-all duration-500">
								100B+
							</h3>
							<p className="text-sm md:text-base font-light text-railway-muted tracking-wide">
								Monthly Requests
							</p>
						</div>
						<div className="stats-glass-card rounded-xl p-8 flex flex-col justify-center items-start group hover:border-indigo-500/30 transition-all duration-300">
							<h3 className="text-5xl md:text-6xl font-[200] text-white tracking-tighter mb-2 group-hover:text-glow-blue transition-all duration-500">
								100M+
							</h3>
							<p className="text-sm md:text-base font-light text-railway-muted tracking-wide">
								Deploys
							</p>
						</div>
					</div>
				</div>
			</main>
			<footer className="border-t hairline-border border-white/5 bg-[#0b0d14] pt-12 pb-8">
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
						<div className="flex flex-col gap-3">
							<span className="text-[10px] font-semibold uppercase tracking-wider text-off-white opacity-50">
								Product
							</span>
							<Link
								className="text-xs text-railway-muted transition-colors hover:text-off-white"
								href="/"
							>
								Features
							</Link>
							<Link
								className="text-xs text-railway-muted transition-colors hover:text-off-white"
								href="/pricing"
							>
								Pricing
							</Link>
						</div>
						<div className="flex flex-col gap-3">
							<span className="text-[10px] font-semibold uppercase tracking-wider text-off-white opacity-50">
								Company
							</span>
							<Link
								className="text-xs text-railway-muted transition-colors hover:text-off-white"
								href="/"
							>
								About
							</Link>
							<Link
								className="text-xs text-railway-muted transition-colors hover:text-off-white"
								href="/careers"
							>
								Careers
							</Link>
						</div>
						<div className="flex flex-col gap-3">
							<span className="text-[10px] font-semibold uppercase tracking-wider text-off-white opacity-50">
								Legal
							</span>
							<Link
								className="text-xs text-railway-muted transition-colors hover:text-off-white"
								href="/privacy"
							>
								Privacy
							</Link>
							<Link
								className="text-xs text-railway-muted transition-colors hover:text-off-white"
								href="/terms"
							>
								Terms
							</Link>
						</div>
					</div>
				</div>
				<div className="mx-auto mt-12 flex max-w-7xl items-center justify-between border-t border-white/5 px-6 pt-8">
					<p className="text-[10px] uppercase text-railway-muted">
						&copy; {currentYear} SpaceScale Inc.
					</p>
					<div className="flex items-center gap-2">
						<span className="size-1.5 rounded-full bg-indigo-500 shadow-[0_0_5px_rgba(99,102,241,0.8)]"></span>
						<span className="text-[10px] font-mono uppercase text-railway-muted">
							Systems Normal
						</span>
					</div>
				</div>
			</footer>
		</div>
	);
}
