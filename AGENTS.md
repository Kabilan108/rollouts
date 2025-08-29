# CLAUDE.md

This file provides guidance to Claude Code (claude.ai/code) when working with code in this repository.

## Project Overview

Dual-purpose infrastructure management system for deploying containerized applications on NixOS:
1. **CLI Tool** (`/cli/`) - "rollout" Go application that generates NixOS container configurations with Traefik reverse proxy
2. **Server Infrastructure** (`/servers/`) - NixOS deployment configuration for the "heighliner" server using deploy-rs

## Development Commands

### Environment Setup
```bash
# Enter development shell with all tools
nix develop

# Enter CLI-focused development shell  
nix develop .#cli
```

### CLI Development (`/cli/`)
```bash
# Build the rollout binary
make build

# Build and run
make run

# Cross-platform build for Linux
make build/rollout-linux-amd64

# Install globally
make install

# Tidy dependencies
make deps

# Create release tarball
make release

# Clean build artifacts
make clean
```

### CLI Usage
```bash
# Interactive mode (TUI)
./build/rollout init

# Interactive dry-run (TUI), prints only the Nix config to stdout
./build/rollout init --dry-run

# Non-interactive mode (requires all required flags)
./build/rollout init --name myapp --image nginx:latest --domain example.com --port 80

# With mounts (binds and named volumes) and secrets
./build/rollout init \
  --name myapp \
  --image nginx:latest \
  --domain example.com \
  --port 80 \
  --network web \
  --mount /var/lib/myapp/data:/data:rw \
  --mount myvol:/cache:ro \
  --env-file ./myapp.env      # or use --edit to open the editor

# Deploy changes with commit message
./build/rollout deploy -m "Add new application"

# Generate GitHub Actions workflow (optional --branch, default: main)
./build/rollout gh-action --branch main
```

### Server Deployment
```bash
# Deploy to heighliner server
deploy .#heighliner

# Build NixOS configuration locally (testing)
nix build .#nixosConfigurations.heighliner.config.system.build.toplevel

# Check deployment configuration
nix flake check

# Build DigitalOcean server image
cd images && nix-build digitalocean.nix
```

### Secrets Management
```bash
# Edit encrypted secrets for an app
agenix -e servers/apps/myapp.age

# Edit system secrets
agenix -e servers/secrets/system.age
```

## Architecture

### CLI Tool (`/cli/`)
Go CLI that generates NixOS container configurations:

**Core Data Structures:**
- `AppConfig`: User input (name, image, domain, port, mounts, env vars)
- `NixAppConfig`: Transforms to NixOS `virtualisation.oci-containers` format  
- `PortRegistry`: JSON-based port allocation tracker (prevents conflicts)

**CLI + TUI:**
- `cli/main.go`: Cobra CLI, non-interactive path, output + file writing
- `cli/tui.go`: Bubble Tea TUI for interactive input (`RunTUI`)

**Key Functions:**
- `(*NixAppConfig).Generate()`: Creates the NixOS configuration template
- `allocatePort()`: Finds next available port in 10000–19999
- `generateAndWriteConfig()`: Writes final config and updates registry/secrets

**Output Path:** `$HOME/repos/rollouts/servers/apps/{name}.nix` (configurable with `--config-dir`)

### Server Infrastructure (`/servers/`)

**Main Configuration** (`heighliner-config.nix`):
- Traefik reverse proxy with Let's Encrypt SSL (Cloudflare DNS challenge)
- Docker with OCI containers backend
- Dynamic app loading from `servers/apps/*.nix`
- Auto-clones dotfiles repository on activation
- SSH key-only authentication
- Firewall: ports 22, 80, 443

**Traefik Setup:**
- Static config: `traefik.yml` (entry points, certificate resolvers)
- Dynamic config: `traefik-dynamic.yml` (middlewares, dashboard auth)
- Automatic HTTP→HTTPS redirect
- www-removal middleware
- Docker provider integration

**Port Registry** (`servers/ports.json`):
- Tracks allocated ports to prevent conflicts
- Updated by CLI when creating new apps
- Format: `{ "app_name": port_number }`

### Integration Flow

1. CLI generates `.nix` file with container config and Traefik labels
2. File written to `servers/apps/` directory  
3. Server's NixOS config automatically imports all app configs
4. Traefik discovers containers via Docker labels
5. SSL certificates obtained automatically via Let's Encrypt

### Container Configuration Format

Generated configs use NixOS `virtualisation.oci-containers.containers` with:
- Image pulling policy: "always" 
- Traefik labels for routing (domain, port, SSL)
- Optional volume mounts and environment variables
- Network mode: bridge with port mappings

### Testing

```bash
# Validate Nix flake configuration
nix flake check

# Build configuration without deploying
nix build .#nixosConfigurations.heighliner.config.system.build.toplevel

# Test CLI locally with dry-run (TUI)
./build/rollout init --dry-run
```

## Code Style Guidelines

### Go Code (`/cli/`)
- Cobra for CLI framework; Bubble Tea for TUI
- TUI is clean and minimal (no emojis), with balanced color usage
- Error messages should be user-friendly with suggestions

### Nix Configurations
- Use `virtualisation.oci-containers` for containers
- Traefik labels follow Docker Compose conventions
- Secrets via agenix with multiple encryption keys
- Atomic deployments with deploy-rs

## Important Files

- `/cli/main.go`: CLI commands, generation, writing, outputs
- `/cli/tui.go`: Interactive TUI (RunTUI)
- `/servers/heighliner-config.nix`: Main server configuration
- `/servers/ports.json`: Port allocation registry
- `/flake.nix`: Build system and deployment configuration
- `/secrets.nix`: Age encryption keys for agenix
