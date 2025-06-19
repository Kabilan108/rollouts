{ config, pkgs, ... }:
{
  virtualisation.oci-containers.containers."toolsdotkabilan108dotcom" = rec {
    image = "ghcr.io/kabilan108/tools.kabilan108.com:latest";
    ports = [ "127.0.0.1:10081:3000" ];
    networks = [ "web" ];
    labels = {
      "traefik.enable" = "true";
      "traefik.docker.network" = "web";
      "traefik.http.services.toolsdotkabilan108dotcom.loadbalancer.server.port" = "3000";

      # domain router
      "traefik.http.routers.toolsdotkabilan108dotcom.rule" = "Host(`tools.kabilan108.com`) || Host(`www.tools.kabilan108.com`)";
      "traefik.http.routers.toolsdotkabilan108dotcom.entrypoints" = "websecure";
      "traefik.http.routers.toolsdotkabilan108dotcom.tls.certresolver" = "letsencrypt";
    };
    environmentFiles = [ config.age.secrets."toolsdotkabilan108dotcom".path ];
  };

  # Force image pull on every deployment
  systemd.services."docker-toolsdotkabilan108dotcom".serviceConfig.ExecStartPre = [
    "${pkgs.docker}/bin/docker pull ghcr.io/kabilan108/tools.kabilan108.com:latest"
  ];
  age.secrets."toolsdotkabilan108dotcom".file = ./toolsdotkabilan108dotcom.age;
}