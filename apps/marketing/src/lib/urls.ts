const DEFAULT_DASHBOARD_URL = "http://localhost:3000";

function sanitizeBaseUrl(url: string): string {
	return url.replace(/\/+$/, "");
}

export function getDashboardUrls() {
	const dashboardBaseUrl = sanitizeBaseUrl(
		process.env.NEXT_PUBLIC_DASHBOARD_URL ?? DEFAULT_DASHBOARD_URL,
	);
	const loginHref = `${dashboardBaseUrl}/login`;
	return { login: loginHref, deploy: loginHref };
}
