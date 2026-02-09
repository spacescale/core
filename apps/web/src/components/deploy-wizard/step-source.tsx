"use client";

import { Github, FileCode, Container } from "lucide-react";
import {
  Card,
  CardContent,
  CardDescription,
  CardHeader,
  CardTitle,
  Input,
  Label,
  Select,
  SelectContent,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from "@/components/ui";
import { cn } from "@/lib/utils";
import { useDeployWizardStore } from "@/stores/deploy-wizard";
import { useGitHubRepos, useGitHubBranches } from "@/lib/hooks";

const sourceTypes = [
  {
    id: "github" as const,
    label: "GitHub Repository",
    description: "Connect your GitHub repo and deploy automatically",
    icon: Github,
  },
  {
    id: "dockerfile" as const,
    label: "Dockerfile",
    description: "Build from a Dockerfile in your repository",
    icon: FileCode,
  },
  {
    id: "docker_image" as const,
    label: "Docker Image",
    description: "Deploy an existing image from a registry",
    icon: Container,
  },
];

export function StepSource() {
  const {
    sourceType,
    setSourceType,
    repository,
    setRepository,
    branch,
    setBranch,
    dockerfilePath,
    setDockerfilePath,
    dockerImage,
    setDockerImage,
    dockerTag,
    setDockerTag,
  } = useDeployWizardStore();

  const { data: repos, isLoading: reposLoading } = useGitHubRepos();

  // Parse owner/repo from repository string
  const [owner, repo] = repository.includes("/") ? repository.split("/") : ["", ""];
  const { data: branches, isLoading: branchesLoading } = useGitHubBranches(owner, repo);

  return (
    <div className="space-y-6">
      <div>
        <h2 className="text-lg font-semibold">Choose a source</h2>
        <p className="text-sm text-muted-foreground">
          Select how you want to deploy your application
        </p>
      </div>

      {/* Source Type Selection */}
      <div className="grid gap-4 md:grid-cols-3">
        {sourceTypes.map((type) => (
          <Card
            key={type.id}
            className={cn(
              "cursor-pointer transition-all hover:border-primary",
              sourceType === type.id && "border-2 border-primary"
            )}
            onClick={() => setSourceType(type.id)}
            role="button"
            tabIndex={0}
            onKeyDown={(e) => e.key === "Enter" && setSourceType(type.id)}
            aria-pressed={sourceType === type.id}
          >
            <CardHeader className="pb-2">
              <type.icon className="h-8 w-8 text-muted-foreground" />
            </CardHeader>
            <CardContent>
              <CardTitle className="text-base">{type.label}</CardTitle>
              <CardDescription className="text-xs">{type.description}</CardDescription>
            </CardContent>
          </Card>
        ))}
      </div>

      {/* GitHub / Dockerfile Options */}
      {(sourceType === "github" || sourceType === "dockerfile") && (
        <div className="space-y-4 rounded-lg border p-4">
          <div className="space-y-2">
            <Label htmlFor="repository">Repository</Label>
            {repos && repos.length > 0 ? (
              <Select value={repository} onValueChange={setRepository}>
                <SelectTrigger id="repository">
                  <SelectValue placeholder="Select a repository" />
                </SelectTrigger>
                <SelectContent>
                  {repos.map((r) => (
                    <SelectItem key={r.id} value={r.fullName}>
                      {r.fullName}
                    </SelectItem>
                  ))}
                </SelectContent>
              </Select>
            ) : (
              <Input
                id="repository"
                placeholder="owner/repository"
                value={repository}
                onChange={(e) => setRepository(e.target.value)}
                aria-describedby="repo-help"
              />
            )}
            <p id="repo-help" className="text-xs text-muted-foreground">
              Enter as owner/repo (e.g., acme/my-app)
            </p>
          </div>

          <div className="space-y-2">
            <Label htmlFor="branch">Branch</Label>
            {branches && branches.length > 0 ? (
              <Select value={branch} onValueChange={setBranch}>
                <SelectTrigger id="branch">
                  <SelectValue placeholder="Select a branch" />
                </SelectTrigger>
                <SelectContent>
                  {branches.map((b) => (
                    <SelectItem key={b.name} value={b.name}>
                      {b.name}
                    </SelectItem>
                  ))}
                </SelectContent>
              </Select>
            ) : (
              <Input
                id="branch"
                placeholder="main"
                value={branch}
                onChange={(e) => setBranch(e.target.value)}
              />
            )}
          </div>

          {sourceType === "dockerfile" && (
            <div className="space-y-2">
              <Label htmlFor="dockerfile-path">Dockerfile Path</Label>
              <Input
                id="dockerfile-path"
                placeholder="Dockerfile"
                value={dockerfilePath}
                onChange={(e) => setDockerfilePath(e.target.value)}
              />
              <p className="text-xs text-muted-foreground">
                Path to your Dockerfile relative to the repository root
              </p>
            </div>
          )}
        </div>
      )}

      {/* Docker Image Options */}
      {sourceType === "docker_image" && (
        <div className="space-y-4 rounded-lg border p-4">
          <div className="space-y-2">
            <Label htmlFor="docker-image">Image</Label>
            <Input
              id="docker-image"
              placeholder="registry/image"
              value={dockerImage}
              onChange={(e) => setDockerImage(e.target.value)}
            />
            <p className="text-xs text-muted-foreground">
              Full image name including registry (e.g., docker.io/library/nginx)
            </p>
          </div>

          <div className="space-y-2">
            <Label htmlFor="docker-tag">Tag</Label>
            <Input
              id="docker-tag"
              placeholder="latest"
              value={dockerTag}
              onChange={(e) => setDockerTag(e.target.value)}
            />
          </div>
        </div>
      )}
    </div>
  );
}
