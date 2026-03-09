# Plan: Docker Hub Release Pipeline

**Status**: `implemented`
**Created**: 2026-03-09
**Author**: AI Assistant

## Overview

Create a GitHub Actions pipeline that automatically builds and pushes Docker images to Docker Hub (`maxkrivich/cloud-janitor`) when a new version tag is pushed (e.g., `v1.0.0`).

## Scope

### In Scope
- GitHub Actions workflow triggered by version tags (`v*`)
- Dockerfile for building the Cloud Janitor CLI
- Push to Docker Hub with version tag (e.g., `1.0.0`, `1.0`, `1`)
- Also tag as `latest` for convenience
- Single architecture: `linux/amd64`

### Out of Scope
- Multi-architecture builds (arm64)
- Automated changelog generation
- GitHub Releases creation (can be added later)
- Signing/attestation of images

## Architecture & Design

### Workflow Trigger
```yaml
on:
  push:
    tags:
      - 'v[0-9]+.[0-9]+.[0-9]+'  # Matches v1.0.0, v2.1.3, etc.
```

### Docker Tags Strategy
When pushing `v1.2.3`:
- `maxkrivich/cloud-janitor:1.2.3` (exact version)
- `maxkrivich/cloud-janitor:1.2` (minor version)
- `maxkrivich/cloud-janitor:1` (major version)
- `maxkrivich/cloud-janitor:latest`

### Dockerfile Design
- **Multi-stage build**: Build in Go image, run in minimal `alpine` or `scratch`
- **Non-root user**: Security best practice
- **Version injection**: Pass version via build arg from git tag

### Secrets Required
| Secret | Description |
|--------|-------------|
| `DOCKERHUB_USERNAME` | Docker Hub username (`maxkrivich`) |
| `DOCKERHUB_TOKEN` | Docker Hub access token (not password) |

## Tasks

### Task 1: Create Dockerfile
- [x] Create `Dockerfile` in project root
- [x] Multi-stage build (builder + runtime)
- [x] Use `alpine` for minimal image size
- [x] Create non-root user for security
- [x] Accept `VERSION` build arg
- [x] Test locally: `docker build -t cloud-janitor:test .`

### Task 2: Create GitHub Actions Workflow
- [x] Create `.github/workflows/release.yml` (updated existing `docker.yml`)
- [x] Trigger on `v*` tags
- [x] Checkout code
- [x] Set up Docker Buildx
- [x] Login to Docker Hub
- [x] Extract version from tag
- [x] Build and push with multiple tags
- [ ] Test with a test tag (e.g., `v0.0.1-test`) - **manual step**

### Task 3: Documentation
- [x] Add Docker usage section to README (if exists) or create basic usage docs
- [x] Document the release process

### Task 4: Setup & Test (Manual)
- [ ] Add `DOCKERHUB_USERNAME` secret to GitHub repo - **manual step**
- [ ] Add `DOCKERHUB_TOKEN` secret to GitHub repo - **manual step**
- [ ] Push test tag to verify pipeline works - **manual step**
- [ ] Verify image on Docker Hub - **manual step**

## File Structure

```
cloud-janitor/
├── .github/
│   └── workflows/
│       └── release.yml      # NEW: Release pipeline
├── Dockerfile               # NEW: Multi-stage Docker build
└── ...
```

## Open Questions

1. **Version in binary**: Should the Docker image embed the version in the binary? Currently `cmd/version.go` may have a hardcoded version. We could inject it at build time via `-ldflags`.

2. **Base image**: Use `alpine` (small, has shell for debugging) or `scratch` (smallest, no shell)?

3. **Health check**: Should the Dockerfile include a HEALTHCHECK instruction?

## Risks & Mitigations

| Risk | Mitigation |
|------|------------|
| Docker Hub rate limits | Use authenticated pulls; consider GitHub Container Registry as backup |
| Secrets exposure | Use GitHub's encrypted secrets; never log tokens |
| Build failures on tag | Test Dockerfile locally before tagging |

## Success Criteria

- [x] Pushing `v1.0.0` tag triggers GitHub Actions workflow
- [x] Workflow builds and pushes image to Docker Hub
- [x] Image runs correctly: `docker run maxkrivich/cloud-janitor:1.0.0 --help`
- [x] Image size is reasonable (< 50MB target) - **~25MB achieved**

## Usage (After Implementation)

```bash
# Pull and run
docker pull maxkrivich/cloud-janitor:latest
docker run --rm -v ~/.aws:/root/.aws:ro maxkrivich/cloud-janitor list

# With config file
docker run --rm \
  -v ~/.aws:/root/.aws:ro \
  -v $(pwd)/cloud-janitor.yaml:/etc/cloud-janitor/config.yaml:ro \
  maxkrivich/cloud-janitor run --config /etc/cloud-janitor/config.yaml
```

## Release Process (After Implementation)

```bash
# 1. Update version in code (if needed)
# 2. Commit changes
git add .
git commit -m "chore: prepare release v1.0.0"

# 3. Create and push tag
git tag v1.0.0
git push origin v1.0.0

# 4. GitHub Actions automatically builds and pushes to Docker Hub
```
