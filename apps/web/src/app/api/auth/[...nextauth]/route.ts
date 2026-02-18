import { createHmac, timingSafeEqual } from "crypto";
import type { Account, NextAuthOptions, User } from "next-auth";
import type { JWT } from "next-auth/jwt";
import NextAuth from "next-auth";
import GithubProvider from "next-auth/providers/github";
import GoogleProvider from "next-auth/providers/google";

const DEFAULT_API_BASE_URL = "http://localhost:8080";
const DEFAULT_BFF_JWT_ISSUER = "spacescale-web-bff";
const DEFAULT_BFF_JWT_AUDIENCE = "spacescale-api";
const DEFAULT_BFF_REFRESH_TOKEN_AUDIENCE = "spacescale-api-refresh";
const DEFAULT_BFF_JWT_TTL_SECONDS = 3600;
const DEFAULT_BFF_REFRESH_TOKEN_TTL_SECONDS = 60 * 60 * 24 * 30;
const API_ACCESS_TOKEN_REFRESH_WINDOW_SECONDS = 60;
const inFlightAccessTokenRefreshByIdentity = new Map<
	string,
	Promise<MintBffAccessTokenResult | null>
>();

type SyncAuthUserResponse = {
	id: string;
	onboardingCompleted: boolean;
};

type MintBffAccessTokenParams = {
	identityKey: string;
	email: string;
	name: string;
	avatarUrl: string;
};

type MintBffAccessTokenResult = {
	value: string;
	expiresAt: number;
};

type MintBffRefreshTokenResult = {
	value: string;
	expiresAt: number;
};

type BffTokenUse = "access" | "refresh";

type BffSignedTokenPayload = {
	sub: string;
	iss: string;
	aud: string;
	iat: number;
	exp: number;
	token_use: BffTokenUse;
	email?: string;
	name?: string;
	avatar_url?: string;
};

type DecodedJwt = {
	header: { alg?: string; typ?: string };
	payload: Record<string, unknown>;
	signingInput: string;
	signature: string;
};

function sanitizeBaseUrl(url: string): string {
	return url.replace(/\/+$/, "");
}

function normalizeEmail(email: string | null | undefined): string {
	return (email ?? "").trim().toLowerCase();
}

function toBase64Url(value: string | Buffer): string {
	return Buffer.from(value)
		.toString("base64")
		.replace(/\+/g, "-")
		.replace(/\//g, "_")
		.replace(/=+$/g, "");
}

function fromBase64Url(value: string): string {
	const base64 = value
		.replace(/-/g, "+")
		.replace(/_/g, "/")
		.padEnd(Math.ceil(value.length / 4) * 4, "=");
	return Buffer.from(base64, "base64").toString("utf8");
}

function safeParseJson(value: string): Record<string, unknown> | null {
	try {
		const parsed = JSON.parse(value);
		return typeof parsed === "object" && parsed !== null
			? (parsed as Record<string, unknown>)
			: null;
	} catch {
		return null;
	}
}

function parsePositiveInteger(
	rawValue: string | undefined,
	fallback: number,
): number {
	const parsed = Number.parseInt((rawValue ?? "").trim(), 10);
	return Number.isFinite(parsed) && parsed > 0 ? parsed : fallback;
}

function decodeJwt(token: string): DecodedJwt | null {
	const parts = token.split(".");
	if (parts.length !== 3) {
		return null;
	}

	const [rawHeader, rawPayload, signature] = parts;
	if (!rawHeader || !rawPayload || !signature) {
		return null;
	}

	const header = safeParseJson(fromBase64Url(rawHeader));
	const payload = safeParseJson(fromBase64Url(rawPayload));
	if (!header || !payload) {
		return null;
	}

	return {
		header: {
			alg: typeof header.alg === "string" ? header.alg : undefined,
			typ: typeof header.typ === "string" ? header.typ : undefined,
		},
		payload,
		signingInput: `${rawHeader}.${rawPayload}`,
		signature,
	};
}

function getBffSigningSecret(): string {
	return (process.env.BFF_JWT_SECRET ?? "").trim();
}

function getRefreshSigningSecret(): string {
	const refreshSecret = (process.env.BFF_REFRESH_TOKEN_SECRET ?? "").trim();
	if (refreshSecret !== "") {
		return refreshSecret;
	}
	return getBffSigningSecret();
}

function signJwt(payload: BffSignedTokenPayload, secret: string): string {
	const header = { alg: "HS256", typ: "JWT" };
	const encodedHeader = toBase64Url(JSON.stringify(header));
	const encodedPayload = toBase64Url(JSON.stringify(payload));
	const signingInput = `${encodedHeader}.${encodedPayload}`;
	const signature = toBase64Url(
		createHmac("sha256", secret).update(signingInput).digest(),
	);
	return `${signingInput}.${signature}`;
}

function buildIdentityKey(account: Account, user: User): string {
	const normalizedEmail = normalizeEmail(user.email);
	if (normalizedEmail !== "") {
		// Email-first identity keeps GitHub/Google sign-in unified for same email.
		return `email:${normalizedEmail}`;
	}

	const provider = (account.provider ?? "oauth").trim().toLowerCase();
	const providerAccountId = (account.providerAccountId ?? "").trim();
	if (providerAccountId !== "") {
		return `${provider}:${providerAccountId}`;
	}

	return `${provider}:unknown`;
}

function buildApiSubject(identityKey: string): string {
	// API currently validates "github:<id>" subject format. We keep this stable
	// while allowing provider-agnostic identity values in the ID segment.
	return `github:${identityKey}`;
}

function getBffIssuer(): string {
	return (process.env.BFF_JWT_ISSUER ?? DEFAULT_BFF_JWT_ISSUER).trim();
}

function getBffAudience(): string {
	return (process.env.BFF_JWT_AUDIENCE ?? DEFAULT_BFF_JWT_AUDIENCE).trim();
}

function getBffRefreshAudience(): string {
	return (
		process.env.BFF_REFRESH_TOKEN_AUDIENCE ?? DEFAULT_BFF_REFRESH_TOKEN_AUDIENCE
	).trim();
}

function shouldRefreshApiAccessToken(expiresAt: number | undefined): boolean {
	if (!expiresAt) {
		return true;
	}

	const now = Math.floor(Date.now() / 1000);
	return now >= expiresAt - API_ACCESS_TOKEN_REFRESH_WINDOW_SECONDS;
}

function mintBffAccessToken({
	identityKey,
	email,
	name,
	avatarUrl,
}: MintBffAccessTokenParams): MintBffAccessTokenResult | null {
	const secret = getBffSigningSecret();
	if (secret === "") {
		return null;
	}

	const issuer = getBffIssuer();
	const audience = getBffAudience();
	const ttlSeconds = parsePositiveInteger(
		process.env.BFF_JWT_TTL_SECONDS,
		DEFAULT_BFF_JWT_TTL_SECONDS,
	);
	const now = Math.floor(Date.now() / 1000);
	const expiresAt = now + ttlSeconds;

	const payload: BffSignedTokenPayload = {
		sub: buildApiSubject(identityKey),
		iss: issuer,
		aud: audience,
		iat: now,
		exp: expiresAt,
		token_use: "access",
	};

	if (email !== "") {
		payload.email = email;
	}
	if (name !== "") {
		payload.name = name;
	}
	if (avatarUrl !== "") {
		payload.avatar_url = avatarUrl;
	}

	return {
		value: signJwt(payload, secret),
		expiresAt,
	};
}

async function mintBffAccessTokenDebounced(
	params: MintBffAccessTokenParams,
): Promise<MintBffAccessTokenResult | null> {
	const identityKey = params.identityKey.trim();
	if (identityKey === "") {
		return null;
	}

	const inFlightRefresh = inFlightAccessTokenRefreshByIdentity.get(identityKey);
	if (inFlightRefresh) {
		return inFlightRefresh;
	}

	const refreshPromise = Promise.resolve()
		.then(() => mintBffAccessToken(params))
		.finally(() => {
			if (inFlightAccessTokenRefreshByIdentity.get(identityKey) === refreshPromise) {
				inFlightAccessTokenRefreshByIdentity.delete(identityKey);
			}
		});

	inFlightAccessTokenRefreshByIdentity.set(identityKey, refreshPromise);
	return refreshPromise;
}

function mintBffRefreshToken(identityKey: string): MintBffRefreshTokenResult | null {
	const secret = getRefreshSigningSecret();
	if (secret === "") {
		return null;
	}

	const ttlSeconds = parsePositiveInteger(
		process.env.BFF_REFRESH_TOKEN_TTL_SECONDS,
		DEFAULT_BFF_REFRESH_TOKEN_TTL_SECONDS,
	);
	const now = Math.floor(Date.now() / 1000);
	const expiresAt = now + ttlSeconds;
	const payload: BffSignedTokenPayload = {
		sub: buildApiSubject(identityKey),
		iss: getBffIssuer(),
		aud: getBffRefreshAudience(),
		iat: now,
		exp: expiresAt,
		token_use: "refresh",
	};

	return {
		value: signJwt(payload, secret),
		expiresAt,
	};
}

function verifyRefreshToken(
	refreshToken: string,
	identityKey: string,
): boolean {
	const secret = getRefreshSigningSecret();
	if (secret === "") {
		return false;
	}

	const decoded = decodeJwt(refreshToken);
	if (!decoded) {
		return false;
	}

	if (decoded.header.alg !== "HS256") {
		return false;
	}

	const expectedSignature = toBase64Url(
		createHmac("sha256", secret).update(decoded.signingInput).digest(),
	);
	const providedSignature = decoded.signature;
	if (providedSignature.length !== expectedSignature.length) {
		return false;
	}
	if (
		!timingSafeEqual(
			Buffer.from(providedSignature),
			Buffer.from(expectedSignature),
		)
	) {
		return false;
	}

	const payload = decoded.payload;
	const now = Math.floor(Date.now() / 1000);

	const subject =
		typeof payload.sub === "string" ? payload.sub.trim() : "";
	const issuer =
		typeof payload.iss === "string" ? payload.iss.trim() : "";
	const audience =
		typeof payload.aud === "string" ? payload.aud.trim() : "";
	const tokenUse =
		typeof payload.token_use === "string" ? payload.token_use.trim() : "";
	const expiresAt =
		typeof payload.exp === "number" ? payload.exp : Number.NaN;

	return (
		subject === buildApiSubject(identityKey) &&
		issuer === getBffIssuer() &&
		audience === getBffRefreshAudience() &&
		tokenUse === "refresh" &&
		Number.isFinite(expiresAt) &&
		expiresAt > now
	);
}

async function syncUserProfile(
	user: User,
	identityKey: string,
): Promise<SyncAuthUserResponse | null> {
	const internalSecret = process.env.INTERNAL_AUTH_SYNC_SECRET ?? "";
	const apiBaseUrl = sanitizeBaseUrl(
		process.env.NEXT_PUBLIC_API_BASE_URL ?? DEFAULT_API_BASE_URL,
	);

	if (internalSecret.trim() === "") {
		return null;
	}

	const response = await fetch(`${apiBaseUrl}/v0/internal/auth-sync`, {
		method: "POST",
		headers: {
			"Content-Type": "application/json",
			"X-Internal-Auth": internalSecret,
		},
		body: JSON.stringify({
			identityKey,
			email: normalizeEmail(user.email),
			name: user.name ?? "",
			avatarUrl: user.image ?? "",
		}),
		cache: "no-store",
	});

	if (!response.ok) {
		throw new Error(`auth-sync failed with status ${response.status}`);
	}

	return (await response.json()) as SyncAuthUserResponse;
}

/**
 * Initializes token fields for a newly authenticated user.
 * Syncs profile to database and mints initial refresh token.
 */
async function initializeNewUserToken(
	token: JWT,
	user: User,
	identityKey: string,
): Promise<void> {
	token.identityKey = identityKey;
	token.profileEmail = normalizeEmail(user.email);
	token.profileName = (user.name ?? "").trim();
	token.profileAvatarUrl = (user.image ?? "").trim();

	try {
		const syncedUser = await syncUserProfile(user, identityKey);

		token.id = syncedUser?.id ?? identityKey;
		token.onboardingCompleted = syncedUser?.onboardingCompleted ?? false;
	} catch (error) {
		// Log the failure with structured data for monitoring/alerting
		console.error("[AUTH CRITICAL] Failed to sync user profile to database", {
			identityKey,
			email: user.email,
			error: error instanceof Error ? error.message : String(error),
			timestamp: new Date().toISOString(),
		});

		// In production, fail auth if database sync fails to prevent data loss
		if (process.env.NODE_ENV === "production") {
			throw new Error(
				"Authentication failed: unable to persist user profile. Please try again.",
			);
		}

		// In development, allow fallback for easier testing
		console.warn(
			"[AUTH WARNING] Allowing auth with fallback values in development mode",
		);
		token.id = identityKey;
		token.onboardingCompleted = false;
	}

	const mintedRefreshToken = mintBffRefreshToken(identityKey);
	if (mintedRefreshToken) {
		token.apiRefreshToken = mintedRefreshToken.value;
		token.apiRefreshTokenExpiresAt = mintedRefreshToken.expiresAt;
	} else {
		console.error(
			"BFF refresh token secret is missing; unable to mint API refresh token",
		);
		token.apiRefreshToken = undefined;
		token.apiRefreshTokenExpiresAt = undefined;
	}
}

/**
 * Ensures all required token fields are populated with valid values.
 * Bootstraps missing fields from existing token data or defaults.
 */
function ensureTokenFieldsPopulated(token: JWT): void {
	// Ensure onboardingCompleted is a boolean
	if (typeof token.onboardingCompleted !== "boolean") {
		token.onboardingCompleted = false;
	}

	// Bootstrap identityKey from email if missing
	if (
		typeof token.identityKey !== "string" ||
		token.identityKey.trim() === ""
	) {
		const tokenEmail = normalizeEmail(
			typeof token.email === "string" ? token.email : null,
		);
		if (tokenEmail !== "") {
			token.identityKey = `email:${tokenEmail}`;
		}
	}

	// Bootstrap profile fields from NextAuth token if missing
	if (typeof token.profileEmail !== "string") {
		token.profileEmail = normalizeEmail(
			typeof token.email === "string" ? token.email : null,
		);
	}
	if (typeof token.profileName !== "string") {
		token.profileName =
			typeof token.name === "string" ? token.name.trim() : "";
	}
	if (typeof token.profileAvatarUrl !== "string") {
		token.profileAvatarUrl =
			typeof token.picture === "string" ? token.picture.trim() : "";
	}

	// Bootstrap ID from identityKey if missing
	if (
		(typeof token.id !== "string" || token.id.trim() === "") &&
		typeof token.identityKey === "string" &&
		token.identityKey.trim() !== ""
	) {
		token.id = token.identityKey;
	}

	// Bootstrap refresh token if missing
	if (
		(typeof token.apiRefreshToken !== "string" ||
			token.apiRefreshToken.trim() === "") &&
		typeof token.identityKey === "string" &&
		token.identityKey.trim() !== ""
	) {
		const bootstrappedRefreshToken = mintBffRefreshToken(token.identityKey);
		if (bootstrappedRefreshToken) {
			token.apiRefreshToken = bootstrappedRefreshToken.value;
			token.apiRefreshTokenExpiresAt = bootstrappedRefreshToken.expiresAt;
		}
	}
}

/**
 * Refreshes the API access token if it's expired or near expiration.
 * Returns early if refresh token is invalid or missing.
 */
async function refreshAccessTokenIfNeeded(token: JWT): Promise<void> {
	if (
		typeof token.identityKey !== "string" ||
		token.identityKey.trim() === "" ||
		!shouldRefreshApiAccessToken(token.apiAccessTokenExpiresAt)
	) {
		return;
	}

	const refreshToken =
		typeof token.apiRefreshToken === "string" ? token.apiRefreshToken : "";

	if (
		refreshToken.trim() === "" ||
		!verifyRefreshToken(refreshToken, token.identityKey)
	) {
		console.error(
			"API refresh token is missing or invalid; unable to mint API access token",
		);
		token.accessToken = undefined;
		token.apiAccessTokenExpiresAt = undefined;
		return;
	}

	const mintedToken = await mintBffAccessTokenDebounced({
		identityKey: token.identityKey,
		email: token.profileEmail ?? "",
		name: token.profileName ?? "",
		avatarUrl: token.profileAvatarUrl ?? "",
	});

	if (mintedToken) {
		token.accessToken = mintedToken.value;
		token.apiAccessTokenExpiresAt = mintedToken.expiresAt;
	} else {
		console.error(
			"BFF_JWT_SECRET is missing; unable to mint API access token",
		);
		token.accessToken = undefined;
		token.apiAccessTokenExpiresAt = undefined;
	}
}

const authOptions: NextAuthOptions = {
	providers: [
		GithubProvider({
			clientId: process.env.GITHUB_CLIENT_ID || "",
			clientSecret: process.env.GITHUB_CLIENT_SECRET || "",
			authorization: {
				params: {
					scope: "read:user user:email repo",
				},
			},
		}),
		GoogleProvider({
			clientId: process.env.GOOGLE_CLIENT_ID || "",
			clientSecret: process.env.GOOGLE_CLIENT_SECRET || "",
		}),
	],
	pages: {
		signIn: "/login",
		error: "/auth/error",
	},
	callbacks: {
		async jwt({ token, account, user }) {
			// Initialize token for newly authenticated users
			if (account && user) {
				const identityKey = buildIdentityKey(account, user);
				await initializeNewUserToken(token, user, identityKey);
			}

			// Ensure all required fields are populated
			ensureTokenFieldsPopulated(token);

			// Refresh access token if needed
			await refreshAccessTokenIfNeeded(token);

			return token;
		},
		async session({ session, token }) {
			if (token) {
				session.accessToken = token.accessToken;
				session.user.id = token.id || "";
				session.user.onboardingCompleted = token.onboardingCompleted === true;
			}
			return session;
		},
	},
	session: {
		strategy: "jwt",
	},
	secret: process.env.NEXTAUTH_SECRET,
};

const handler = NextAuth(authOptions);

export { handler as GET, handler as POST };
