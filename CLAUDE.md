# CLAUDE.md

This file provides guidance to Claude Code (claude.ai/code) when working with code in this repository.

## Project Overview

This is a dual-purpose repository containing:
1. **CLI Tool** (`/cli/`) - "rollout" Go application that generates NixOS container configurations with Traefik reverse proxy
2. **Server Infrastructure** (`/servers/`) - NixOS deployment configuration for the "heighliner" server using deploy-rs

## Key Commands

### Development Environment
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
```

### CLI Usage
```bash
# Interactive mode to create app config
./build/rollout init

# Non-interactive mode with flags
./build/rollout init --name myapp --image nginx:latest --domain example.com --port 80
```

### Server Deployment
```bash
# Deploy to heighliner server
deploy .#heighliner

# Build NixOS configuration locally (testing)
nix build .#nixosConfigurations.heighliner.config.system.build.toplevel

# Check deployment configuration
nix flake check
```

### Image Building
```bash
# Build DigitalOcean server image
cd images && nix-build digitalocean.nix
```

## Architecture

### CLI Tool (`/cli/main.go`)
Single-file Go application that generates NixOS container configurations:
- **Core Structs**: `AppConfig` (user input) → `NixAppConfig` (NixOS format)
- **UI Framework**: Bubble Tea for interactive TUI, supports both interactive and flag-based usage
- **Output**: Generates `virtualisation.oci-containers` configs with Traefik labels
- **Target Path**: Writes to `$HOME/dotfiles/servers/apps/{name}.nix` by default

### Server Infrastructure
**Main Configuration**: `servers/heighliner-config.nix` - Complete NixOS server setup with:
- **Traefik**: Reverse proxy with Let's Encrypt SSL, Docker provider integration
- **Container Runtime**: Docker with OCI containers backend
- **App Loading**: Dynamically imports all `.nix` files from `servers/apps/` directory
- **Dotfiles Integration**: Auto-clones and symlinks dotfiles on system activation
- **Security**: SSH key-only authentication, firewall configured for ports 22, 80, 443

**Deployment**: Uses deploy-rs for atomic deployments with rollback capability to root@heighliner

### Key Integration Points
- CLI generates app configs that server automatically loads from `servers/apps/`
- Traefik labels in CLI output integrate with server's Traefik configuration
- Both CLI and server use same NixOS `virtualisation.oci-containers` format

### Secrets Management
- Uses agenix for encrypted secrets
- Environment variables loaded into Traefik service from `secrets/system.age`