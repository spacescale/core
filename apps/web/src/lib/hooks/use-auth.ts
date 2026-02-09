"use client";

import { useSession, signIn, signOut } from "next-auth/react";
import { useRouter } from "next/navigation";
import { useCallback } from "react";

export function useAuth() {
  const { data: session, status } = useSession();
  const router = useRouter();

  const isLoading = status === "loading";
  const isAuthenticated = status === "authenticated";
  const isUnauthenticated = status === "unauthenticated";

  const loginWithGithub = useCallback(async () => {
    await signIn("github", { callbackUrl: "/app" });
  }, []);

  const loginWithGoogle = useCallback(async () => {
    await signIn("google", { callbackUrl: "/app" });
  }, []);

  const logout = useCallback(async () => {
    await signOut({ callbackUrl: "/login" });
  }, []);

  const requireAuth = useCallback(() => {
    if (isUnauthenticated) {
      router.push("/login");
      return false;
    }
    return isAuthenticated;
  }, [isAuthenticated, isUnauthenticated, router]);

  return {
    session,
    user: session?.user,
    accessToken: session?.accessToken,
    isLoading,
    isAuthenticated,
    isUnauthenticated,
    loginWithGithub,
    loginWithGoogle,
    logout,
    requireAuth,
  };
}
