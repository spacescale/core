type CapturedAuthOptions = {
  callbacks?: {
    jwt?: (params: Record<string, unknown>) => Promise<unknown>;
    session?: (params: Record<string, unknown>) => Promise<unknown>;
  };
};

type NextAuthGlobal = typeof globalThis & {
  __capturedNextAuthOptions?: CapturedAuthOptions;
};

export default function NextAuth(options: unknown): () => void {
  (globalThis as NextAuthGlobal).__capturedNextAuthOptions =
    options as CapturedAuthOptions;
  return () => {};
}
