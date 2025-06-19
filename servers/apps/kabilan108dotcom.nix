{ config, pkgs, ... }:
{
  virtualisation.oci-containers.containers."kabilan108dotcom" = rec {
    image = "ghcr.io/kabilan108/kabilan108.com:latest";
    ports = [ "127.0.0.1:10080:80" ];
    networks = [ "web" ];
    labels = {
      "traefik.enable" = "true";
      "traefik.docker.network" = "web";
      "traefik.http.services.kabilan108dotcom.loadbalancer.server.port" = "80";

      # domain router
      "traefik.http.routers.kabilan108dotcom.rule" = "Host(`kabilan108.com`) || Host(`www.kabilan108.com`)";
      "traefik.http.routers.kabilan108dotcom.entrypoints" = "websecure";
      "traefik.http.routers.kabilan108dotcom.tls.certresolver" = "letsencrypt";
    };
  };

  # Force image pull on every deployment
  systemd.services."docker-kabilan108dotcom".serviceConfig.ExecStartPre = [
    "${pkgs.docker}/bin/docker pull ghcr.io/kabilan108/kabilan108.com:latest"
  ];
}