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
- `./build/rollout init` - Interactive mode to create app config
- `./build/rollout init --name myapp --image nginx:latest --domain example.com --port 80` - Non-interactive mode

## Architecture

**Core Components:**
- `main.go` - Single-file application containing all logic
- `AppConfig` struct - Holds user configuration (name, image, domain, port, etc.)
- `NixAppConfig` struct - Transforms user config into NixOS container configuration
- `model` struct - Bubble Tea TUI model for interactive input

**Key Functions:**
- `Generate()` method on `NixAppConfig` - Generates the NixOS configuration template
- `runInitWithAppConfig()` - Orchestrates the interactive TUI or direct config generation
- `generateAndWriteConfig()` - Outputs final config and writes to file system

**UI Framework:**
- Uses Charm libraries: Bubble Tea (TUI), Bubbles (components), Lipgloss (styling)
- Interactive prompts collect missing configuration values
- Supports both interactive and flag-based CLI usage

**Output:**
- Generates NixOS `virtualisation.oci-containers` configuration
- Includes Traefik labels for reverse proxy routing
- Writes to `$HOME/dotfiles/servers/apps/{name}.nix` by default
- Supports `--dry-run` flag to preview without writing

**Dependencies:**
- Cobra for CLI framework
- Charm suite for TUI
- Standard Go libraries for file operations
- No external runtime dependencies