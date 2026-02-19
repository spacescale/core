import type { Account, NextAuthOptions, User } from "next-auth";
import type { JWT } from "next-auth/jwt";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";

type JwtCallback = NonNullable<
  NonNullable<NextAuthOptions["callbacks"]>["jwt"]
>;
type SessionCallback = NonNullable<
  NonNullable<NextAuthOptions["callbacks"]>["session"]
>;

let capturedAuthOptions: NextAuthOptions | undefined;

vi.mock("next-auth", () => ({
  default: vi.fn((options: NextAuthOptions) => {
    capturedAuthOptions = options;
    return vi.fn();
  }),
}), { virtual: true });

vi.mock("next-auth/providers/github", () => ({
  default: vi.fn(() => ({ id: "github", name: "GitHub", type: "oauth" })),
}), { virtual: true });

vi.mock("next-auth/providers/google", () => ({
  default: vi.fn(() => ({ id: "google", name: "Google", type: "oauth" })),
}), { virtual: true });

const ORIGINAL_ENV = { ...process.env };

function getFetchMock() {
  return global.fetch as unknown as ReturnType<typeof vi.fn>;
}

function resetEnv(): void {
  for (const key of Object.keys(process.env)) {
    delete process.env[key];
  }
  Object.assign(process.env, ORIGINAL_ENV);
}

function setEnv(overrides: Record<string, string | undefined>): void {
  for (const [key, value] of Object.entries(overrides)) {
    if (value === undefined) {
      delete process.env[key];
      continue;
    }
    process.env[key] = value;
  }
}

function oauthAccount(provider: string, providerAccountId: string): Account {
  return {
    provider,
    providerAccountId,
    type: "oauth",
  } as Account;
}

function oauthUser(email: string): User {
  return {
    id: "next-auth-user",
    email,
    name: "Test User",
    image: "https://example.com/avatar.png",
  };
}

async function loadAuthCallbacks(
  overrides: Record<string, string | undefined> = {},
): Promise<{ jwt: JwtCallback; session: SessionCallback }> {
  vi.resetModules();
  capturedAuthOptions = undefined;

  setEnv({
    NODE_ENV: "test",
    NEXTAUTH_SECRET: "test-nextauth-secret",
    NEXT_PUBLIC_API_BASE_URL: "http://localhost:8080",
    INTERNAL_AUTH_SYNC_SECRET: "test-internal-secret",
    BFF_JWT_SECRET: "test-bff-access-secret",
    BFF_REFRESH_TOKEN_SECRET: "test-bff-refresh-secret",
    GITHUB_CLIENT_ID: "test-github-id",
    GITHUB_CLIENT_SECRET: "test-github-secret",
    GOOGLE_CLIENT_ID: "test-google-id",
    GOOGLE_CLIENT_SECRET: "test-google-secret",
  });
  setEnv(overrides);

  await import("@/app/api/auth/[...nextauth]/route");

  if (!capturedAuthOptions?.callbacks?.jwt || !capturedAuthOptions.callbacks.session) {
    throw new Error("failed to capture NextAuth callbacks");
  }

  return {
    jwt: capturedAuthOptions.callbacks.jwt as JwtCallback,
    session: capturedAuthOptions.callbacks.session as SessionCallback,
  };
}

describe("NextAuth route callbacks", () => {
  beforeEach(() => {
    vi.clearAllMocks();
    resetEnv();
    vi.stubGlobal("fetch", vi.fn());
  });

  afterEach(() => {
    vi.unstubAllGlobals();
    resetEnv();
  });

  it("fails sign-in in production when INTERNAL_AUTH_SYNC_SECRET is missing", async () => {
    const { jwt } = await loadAuthCallbacks({
      NODE_ENV: "production",
      INTERNAL_AUTH_SYNC_SECRET: undefined,
    });

    const token = {} as JWT;
    await expect(
      jwt({
        token,
        account: oauthAccount("github", "12345"),
        user: oauthUser("person@example.com"),
      } as Parameters<JwtCallback>[0]),
    ).rejects.toThrow("unable to persist user profile");

    expect(getFetchMock()).not.toHaveBeenCalled();
    expect(token.identityKey).toBeUndefined();
    expect(token.id).toBeUndefined();
    expect(token.apiRefreshToken).toBeUndefined();
    expect(token.accessToken).toBeUndefined();
  });

  it("fails sign-in in production when refresh-token secret is missing", async () => {
    getFetchMock().mockResolvedValue({
      ok: true,
      json: async () => ({ id: "user-1", onboardingCompleted: true }),
    } as Response);

    const { jwt } = await loadAuthCallbacks({
      NODE_ENV: "production",
      BFF_REFRESH_TOKEN_SECRET: undefined,
    });

    const token = {} as JWT;
    await expect(
      jwt({
        token,
        account: oauthAccount("google", "abc-123"),
        user: oauthUser("person@example.com"),
      } as Parameters<JwtCallback>[0]),
    ).rejects.toThrow("unable to issue API refresh token");

    expect(getFetchMock()).toHaveBeenCalledTimes(1);
    expect(token.identityKey).toBeUndefined();
    expect(token.apiRefreshToken).toBeUndefined();
    expect(token.accessToken).toBeUndefined();
  });

  it("hard-fails when access token cannot be minted (missing BFF_JWT_SECRET)", async () => {
    const { jwt } = await loadAuthCallbacks({
      NODE_ENV: "production",
      BFF_JWT_SECRET: undefined,
    });

    const token = {
      identityKey: "email:person@example.com",
      profileEmail: "person@example.com",
      profileName: "Test User",
      profileAvatarUrl: "https://example.com/avatar.png",
    } as JWT;

    await expect(jwt({ token } as Parameters<JwtCallback>[0])).rejects.toThrow(
      "unable to mint API access token",
    );

    expect(token.identityKey).toBeUndefined();
    expect(token.apiRefreshToken).toBeUndefined();
    expect(token.accessToken).toBeUndefined();
  });

  it("hard-fails when refresh token is invalid", async () => {
    const { jwt } = await loadAuthCallbacks({
      NODE_ENV: "production",
    });

    const token = {
      identityKey: "email:person@example.com",
      profileEmail: "person@example.com",
      profileName: "Test User",
      profileAvatarUrl: "https://example.com/avatar.png",
      apiRefreshToken: "not-a-valid-refresh-token",
      apiAccessTokenExpiresAt: 0,
    } as JWT;

    await expect(jwt({ token } as Parameters<JwtCallback>[0])).rejects.toThrow(
      "unable to refresh API access token",
    );

    expect(token.identityKey).toBeUndefined();
    expect(token.apiRefreshToken).toBeUndefined();
    expect(token.accessToken).toBeUndefined();
  });

  it("throws in session callback when API access token is missing", async () => {
    const { session } = await loadAuthCallbacks();

    await expect(
      session({
        session: {
          expires: "2099-01-01T00:00:00.000Z",
          user: { name: null, email: null, image: null },
        },
        token: {},
      } as Parameters<SessionCallback>[0]),
    ).rejects.toThrow("Session invalid: missing API access token");
  });

  it("initializes session.user when undefined and writes id/onboarding fields", async () => {
    const { session } = await loadAuthCallbacks();

    const result = await session({
      session: {
        expires: "2099-01-01T00:00:00.000Z",
        user: undefined,
      },
      token: {
        accessToken: "access-token",
        id: "user-123",
        onboardingCompleted: true,
      },
    } as Parameters<SessionCallback>[0]);

    expect(result.accessToken).toBe("access-token");
    expect(result.user).toBeDefined();
    expect(result.user.id).toBe("user-123");
    expect(result.user.onboardingCompleted).toBe(true);
  });

  it("redacts email from auth-sync failure logs", async () => {
    const consoleErrorSpy = vi
      .spyOn(console, "error")
      .mockImplementation(() => undefined);

    getFetchMock().mockResolvedValue({
      ok: false,
      status: 500,
    } as Response);

    const { jwt } = await loadAuthCallbacks({
      NODE_ENV: "production",
    });

    await expect(
      jwt({
        token: {} as JWT,
        account: oauthAccount("github", "12345"),
        user: oauthUser("person@example.com"),
      } as Parameters<JwtCallback>[0]),
    ).rejects.toThrow("unable to persist user profile");

    const authLogCall = consoleErrorSpy.mock.calls.find((call) =>
      String(call[0]).includes("[AUTH CRITICAL]"),
    );
    expect(authLogCall).toBeDefined();
    const metadata = authLogCall?.[1] as Record<string, unknown>;
    expect(metadata.identityKey).toBe("email:person@example.com");
    expect(metadata).not.toHaveProperty("email");
  });
});
