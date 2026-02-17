import type { DefaultSession } from "next-auth";

declare module "next-auth" {
  interface Session {
    accessToken?: string;
    user: DefaultSession["user"] & {
      id: string;
      onboardingCompleted: boolean;
    };
  }
}

declare module "next-auth/jwt" {
  interface JWT {
    accessToken?: string;
    id?: string;
    onboardingCompleted?: boolean;
    identityKey?: string;
    profileEmail?: string;
    profileName?: string;
    profileAvatarUrl?: string;
    apiAccessTokenExpiresAt?: number;
    apiRefreshToken?: string;
    apiRefreshTokenExpiresAt?: number;
  }
}
