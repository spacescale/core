type OAuthProviderConfig = {
  id: string;
  name: string;
  type: "oauth";
};

export default function GithubProvider(): OAuthProviderConfig {
  return {
    id: "github",
    name: "GitHub",
    type: "oauth",
  };
}
