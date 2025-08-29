# CLAUDE.md

This file provides guidance to Claude Code (claude.ai/code) when working with code in this repository.

## Project Overview

This is "rollout", a Go CLI tool that generates NixOS configurations for containerized applications with Traefik reverse proxy. It creates OCI container configurations with proper Traefik labels for domain routing and SSL termination via Let's Encrypt.

## Key Commands

**Build and Development:**
- `make build` - Build the rollout binary to `build/rollout`
- `make run` - Build and run the application
- `make install` - Install globally with `go install`
- `make deps` - Tidy Go module dependencies

**Cross-platform Build:**
- `make build/rollout-linux-amd64` - Build Linux AMD64 binary
- `make release` - Create tarball release for Linux AMD64

**Nix (if using Nix environment):**
- `nix build` - Build using Nix flake
- `nix develop` - Enter development shell with Go toolchain

**Usage:**
- `./build/rollout init` - Interactive TUI to create app config
- `./build/rollout init --dry-run` - Interactive TUI, prints only the generated Nix config to stdout
- `./build/rollout init --name myapp --image nginx:latest --domain example.com --port 80` - Non-interactive mode (requires all required flags)
- Optional flags (both TUI and non-interactive): `--subdomain`, `--network`, `--mount`, `--env-file` or `--edit`
- `./build/rollout gh-action --branch main` - Print GitHub Actions workflow (branch optional, default: main)

## Architecture

**Core Components:**
- `main.go` - Cobra CLI, non-interactive flow, generation/writing, outputs
- `tui.go` - Bubble Tea TUI (`RunTUI`) for collecting interactive input
- `AppConfig` - User input (name, image, domain, port, mounts, env vars)
- `NixAppConfig` - Transforms user config into NixOS container configuration

**Key Functions:**
- `(*NixAppConfig).Generate()` - Generates the NixOS configuration template
- `generateAndWriteConfig()` - Writes final config, updates port registry, and handles secrets

**UI Framework:**
- Uses Charm libraries: Bubble Tea (TUI) and Lipgloss (styling)
- Clean, straightforward TUI (no emojis), balanced color usage
- Interactive mode is used only when running `rollout init` (or `--dry-run`)

**TUI Fields:**
- Name, Image, Domain, Subdomain (optional), Container Port, Network (default: web)
- Secrets mode: `none | file | edit`
  - `file`: prompts for path and encrypts with agenix
  - `edit`: opens agenix editor after write
- Mounts: comma-separated list, e.g. `/host:/container:rw, name:/container:ro`

**Output:**
- Generates NixOS `virtualisation.oci-containers` configuration
- Includes Traefik labels for reverse proxy routing
- Writes to `$HOME/repos/rollouts/servers/apps/{name}.nix` by default (override with `--config-dir`)
- `--dry-run` prints only the raw configuration to stdout (no summary)
- Normal runs print a concise summary (no bounding box, no full config)

**Dependencies:**
- Cobra for CLI framework
- Charm suite for TUI
- Standard Go libraries for file operations
- `agenix` required at runtime for `--env-file` or `--edit` flows
