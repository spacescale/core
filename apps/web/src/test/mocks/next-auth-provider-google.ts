type OAuthProviderConfig = {
  id: string;
  name: string;
  type: "oauth";
};

export default function GoogleProvider(): OAuthProviderConfig {
  return {
    id: "google",
    name: "Google",
    type: "oauth",
  };
}
