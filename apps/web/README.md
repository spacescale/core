# PaaS Web Frontend

Production-quality frontend for the Platform-as-a-Service MVP. Built with Next.js 15, TypeScript, Tailwind CSS, and React Query.

## Features

- **Authentication**: GitHub and Google OAuth via NextAuth.js
- **Dashboard**: View and manage all deployed applications
- **Deploy Wizard**: Multi-step deployment flow with draft persistence
- **Deployment Monitoring**: Real-time deployment progress with timeline and logs
- **App Management**: Overview, deployments, logs, and settings tabs
- **Mock Mode**: Full mock API for development and testing
- **Responsive Design**: Mobile, tablet, and desktop layouts

## Tech Stack

- **Framework**: Next.js 15 (App Router)
- **Language**: TypeScript
- **Styling**: Tailwind CSS + Radix UI components
- **Data Fetching**: TanStack Query (React Query)
- **Validation**: Zod schemas
- **State Management**: Zustand
- **Authentication**: NextAuth.js
- **Testing**: Vitest + React Testing Library

## Getting Started

### Prerequisites

- Node.js 18+
- pnpm (recommended) or npm

### Installation

```bash
# From the monorepo root
pnpm install

# Or from this directory
cd apps/web
pnpm install
```

### Environment Setup

Copy the example environment file:

```bash
cp .env.example .env.local
```

Configure the following variables in `.env.local`:

```env
# API Configuration
NEXT_PUBLIC_API_BASE_URL=http://localhost:8080
NEXT_PUBLIC_USE_MOCK_API=true  # Set to false for real API

# OAuth Configuration
NEXTAUTH_SECRET=your-secret-here  # Generate: openssl rand -base64 32
NEXTAUTH_URL=http://localhost:3000

# GitHub OAuth (create at https://github.com/settings/developers)
GITHUB_CLIENT_ID=your-github-client-id
GITHUB_CLIENT_SECRET=your-github-client-secret

# Google OAuth (create at https://console.cloud.google.com/apis/credentials)
GOOGLE_CLIENT_ID=your-google-client-id
GOOGLE_CLIENT_SECRET=your-google-client-secret

# Platform
NEXT_PUBLIC_PLATFORM_DOMAIN=ourplatform.io
```

### OAuth Setup

#### GitHub

1. Go to [GitHub Developer Settings](https://github.com/settings/developers)
2. Click "New OAuth App"
3. Set the callback URL to: `http://localhost:3000/api/auth/callback/github`
4. Copy the Client ID and Client Secret to your `.env.local`

#### Google

1. Go to [Google Cloud Console](https://console.cloud.google.com/apis/credentials)
2. Create a new OAuth 2.0 Client ID
3. Set the authorized redirect URI to: `http://localhost:3000/api/auth/callback/google`
4. Copy the Client ID and Client Secret to your `.env.local`

### Running the App

```bash
# Development mode
pnpm dev

# Production build
pnpm build
pnpm start
```

Open [http://localhost:3000](http://localhost:3000) in your browser.

## Mock Mode vs Real Mode

### Mock Mode (Default)

When `NEXT_PUBLIC_USE_MOCK_API=true`, the app uses simulated API responses:

- All API calls return mock data from `src/mocks/`
- Deployments simulate progress through stages automatically
- Subdomain availability checks work with preset data
- No backend required

### Real Mode

When `NEXT_PUBLIC_USE_MOCK_API=false`:

- All API calls go to `NEXT_PUBLIC_API_BASE_URL`
- Requires a running backend API
- Authentication tokens are passed via Authorization header

## Where to Edit Endpoints

All API endpoint paths are defined in a single file:

```
src/lib/api/endpoints.ts
```

Update this file to match your backend API structure. The API client (`src/lib/api/client.ts`) uses these paths automatically.

## Project Structure

```
src/
├── app/                    # Next.js App Router pages
│   ├── (authenticated)/    # Protected routes (requires auth)
│   │   └── app/           # Main app pages
│   │       ├── page.tsx   # Dashboard
│   │       ├── new/       # Deploy wizard
│   │       ├── apps/      # App detail pages
│   │       └── deployments/# Deployment progress
│   ├── auth/              # Auth error page
│   ├── login/             # Login page
│   └── layout.tsx         # Root layout
├── components/
│   ├── ui/                # Design system components
│   ├── layout/            # Shell, sidebar, header
│   ├── deploy-wizard/     # Wizard step components
│   └── app-detail/        # App detail tab components
├── lib/
│   ├── api/               # API client and endpoints
│   ├── auth/              # Auth session provider
│   ├── hooks/             # React hooks (data fetching)
│   ├── providers/         # React Query provider
│   ├── schemas/           # Zod validation schemas
│   └── utils/             # Utility functions
├── mocks/                 # Mock API handlers and data
├── stores/                # Zustand stores
└── __tests__/             # Test files
```

## Available Scripts

```bash
pnpm dev          # Start development server
pnpm build        # Build for production
pnpm start        # Start production server
pnpm lint         # Run ESLint
pnpm format       # Format code with Prettier
pnpm format:check # Check formatting
pnpm test         # Run tests
pnpm test:ui      # Run tests with UI
pnpm test:coverage # Run tests with coverage
pnpm typecheck    # Type check without emitting
```

## API Contract

The frontend expects the following API endpoints. See `src/lib/api/endpoints.ts` for the full list.

### Apps

- `GET /v1/apps` - List all apps
- `GET /v1/apps/:appId` - Get app details
- `PATCH /v1/apps/:appId` - Update app
- `DELETE /v1/apps/:appId` - Delete app
- `POST /v1/apps/:appId/redeploy` - Trigger redeploy
- `GET /v1/apps/:appId/deployments` - List app deployments
- `GET /v1/apps/:appId/logs` - Get app logs

### Deployments

- `POST /v1/deployments` - Create deployment
- `GET /v1/deployments/:deploymentId` - Get deployment
- `POST /v1/deployments/:deploymentId/cancel` - Cancel deployment
- `POST /v1/deployments/:deploymentId/retry` - Retry deployment
- `GET /v1/deployments/:deploymentId/logs` - Get deployment logs

### Subdomains

- `GET /v1/subdomains/check?name=:subdomain` - Check availability

### GitHub (optional)

- `GET /v1/github/repos` - List user repos
- `GET /v1/github/repos/:owner/:repo/branches` - List branches

## Design System

The UI uses a custom design system built on Radix UI primitives:

- **Button**: Primary actions with loading states
- **Input**: Text inputs with error states
- **Select**: Dropdown selects
- **Badge**: Status indicators
- **Card**: Content containers
- **Dialog**: Modals and confirmations
- **Toast**: Notifications
- **Tabs**: Tab navigation
- **Skeleton**: Loading placeholders

## Known Assumptions

1. **Authentication**: NextAuth handles OAuth; backend may need token exchange endpoint
2. **GitHub Integration**: If GitHub repo listing API is unavailable, users can manually enter `owner/repo`
3. **Deployment Steps**: The timeline assumes a fixed set of deployment stages
4. **Logs**: Logs are polled every 5 seconds (not streamed via WebSocket)
5. **Plans**: Plan selection is UI-only; no pricing/billing integration

## Troubleshooting

### "Module not found" errors

Ensure all dependencies are installed:
```bash
pnpm install
```

### OAuth redirect errors

1. Verify callback URLs match in OAuth provider settings
2. Check `NEXTAUTH_URL` matches your local URL
3. Ensure `NEXTAUTH_SECRET` is set

### Mock API not working

1. Verify `NEXT_PUBLIC_USE_MOCK_API=true` in `.env.local`
2. Restart the dev server after changing env vars

### Build errors

Run type checking to identify issues:
```bash
pnpm typecheck
```

## License

Private - All rights reserved
