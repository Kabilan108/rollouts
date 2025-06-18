{
  description = "Rollouts - CLI tool and server deployment for containerized applications";

  inputs = {
    nixpkgs.url = "github:NixOS/nixpkgs/nixos-unstable";
    deploy-rs.url = "github:serokell/deploy-rs";
    agenix.url = "github:ryantm/agenix";
  };

  outputs =
    {
      self,
      nixpkgs,
      deploy-rs,
      agenix,
    }:
    let
      system = "x86_64-linux";
      pkgs = import nixpkgs { inherit system; };

      deployPkgs = import nixpkgs {
        inherit system;
        overlays = [
          deploy-rs.overlays.default
          (final: prev: {
            deploy-rs = {
              inherit (pkgs) deploy-rs;
              lib = prev.deploy-rs.lib;
            };
          })
        ];
      };

      rolloutCli = pkgs.buildGoModule rec {
        pname = "rollout";
        version = "latest";
        src = ./cli;

        vendorHash = "sha256-pn603h6eS1TOatadmfJJMkxTbxdu6zhRpAhFAu43ors=";

        buildPhase = ''
          runHook preBuild
          make build
          runHook postBuild
        '';

        installPhase = ''
          runHook preInstall
          mkdir -p $out/bin
          cp build/rollout $out/bin/
          runHook postInstall
        '';

        meta = with pkgs.lib; {
          description = "CLI tool for generating NixOS container configurations with Traefik";
          homepage = "https://github.com/kabilan108/rollouts";
          license = licenses.mit;
          maintainers = [ "kabilan108" ];
        };
      };
    in
    {
      # cli package - installable with `nix install github:kabilan108/rollouts#cli`
      packages.${system} = {
        cli = rolloutCli;
        default = rolloutCli; # `nix install github:kabilan108/rollouts`
        agenix = agenix.packages.${system}.default;
      };

      # server configurations
      nixosConfigurations.heighliner = nixpkgs.lib.nixosSystem {
        inherit system;
        modules = [
          ./servers/heighliner-config.nix
          agenix.nixosModules.default
        ];
      };

      # deploy-rs configuration
      deploy.nodes.heighliner = {
        hostname = "heighliner";
        sshUser = "root";
        profiles.system = {
          user = "root";
          path = deployPkgs.deploy-rs.lib.activate.nixos self.nixosConfigurations.heighliner;
        };
      };

      # deployment checks
      checks = builtins.mapAttrs (system: deployLib: deployLib.deployChecks self.deploy) deploy-rs.lib;

      devShells.${system} = {
        cli = pkgs.mkShell {
          buildInputs = with pkgs; [
            go
            gopls
            nodejs_20
          ];
          shellHook = ''
            export NPM_CONFIG_PREFIX="$HOME/.npm-global"
            export PATH="$HOME/.npm-global/bin:$PATH"
            if [ ! -f "$HOME/.npm-global/bin/claude" ]; then
              npm install -g @anthropic-ai/claude-code
            fi
          '';
        };

        default = pkgs.mkShell {
          buildInputs = with pkgs; [
            rolloutCli
            go
            gopls
            agenix.packages.${system}.default
            deploy-rs.packages.${system}.default
            cachix
            nodejs_20
          ];
          shellHook = ''
            export NPM_CONFIG_PREFIX="$HOME/.npm-global"
            export PATH="$HOME/.npm-global/bin:$PATH"
            if [ ! -f "$HOME/.npm-global/bin/claude" ]; then
              npm install -g @anthropic-ai/claude-code
            fi
          '';
        };
      };

      images.${system}.digitalocean =
        (pkgs.nixos {
          imports = [ ./images/digitalocean.nix ];
          system.stateVersion = "25.05";
        }).digitalOceanImage;
    };
}
