"use client";

import { Suspense } from "react";
import { useSearchParams } from "next/navigation";
import Link from "next/link";
import { AlertCircle } from "lucide-react";
import { Card, CardContent, CardHeader, CardTitle, Skeleton } from "@/components/ui";

const errorMessages: Record<string, string> = {
  Configuration: "There is a problem with the server configuration.",
  AccessDenied: "You do not have permission to sign in.",
  Verification: "The verification link may have expired or already been used.",
  OAuthSignin: "Error occurred during OAuth sign in.",
  OAuthCallback: "Error occurred during OAuth callback.",
  OAuthCreateAccount: "Could not create OAuth provider account.",
  EmailCreateAccount: "Could not create email provider account.",
  Callback: "Error occurred during callback.",
  OAuthAccountNotLinked: "This email is already associated with a different account.",
  EmailSignin: "Error sending the verification email.",
  CredentialsSignin: "Sign in failed. Check your credentials.",
  SessionRequired: "You must be signed in to access this page.",
  Default: "An unexpected error occurred.",
};

function AuthErrorContent() {
  const searchParams = useSearchParams();
  const error = searchParams.get("error");
  const errorMessage = error ? errorMessages[error] || errorMessages.Default : errorMessages.Default;

  return (
    <p className="text-sm text-muted-foreground">{errorMessage}</p>
  );
}

function AuthErrorFallback() {
  return <Skeleton className="h-4 w-48 mx-auto" />;
}

export default function AuthErrorPage() {
  return (
    <div className="flex min-h-screen items-center justify-center bg-gradient-to-br from-background to-muted p-4">
      <Card className="w-full max-w-md">
        <CardHeader className="text-center">
          <div className="mx-auto mb-4 flex h-12 w-12 items-center justify-center rounded-full bg-destructive/10">
            <AlertCircle className="h-6 w-6 text-destructive" />
          </div>
          <CardTitle className="text-2xl">Authentication Error</CardTitle>
          <Suspense fallback={<AuthErrorFallback />}>
            <AuthErrorContent />
          </Suspense>
        </CardHeader>
        <CardContent className="space-y-4">
          <Link
            href="/login"
            className="inline-flex h-10 w-full items-center justify-center rounded-md bg-primary px-4 py-2 text-sm font-medium text-primary-foreground ring-offset-background transition-colors hover:bg-primary/90 focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-ring focus-visible:ring-offset-2"
          >
            Try Again
          </Link>
          <p className="text-center text-xs text-muted-foreground">
            If this problem persists, please contact support.
            <br />
            <a href="#" className="underline hover:text-foreground">
              Report an issue
            </a>
          </p>
        </CardContent>
      </Card>
    </div>
  );
}
