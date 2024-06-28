app:
{ config, lib, pkgs, ... }:
{

    security.acme = {
    acceptTerms = true;
    defaults.email = "rwendt1337@gmail.com";
    certs = {
      "flakery.xyz" = {
        domain = domain;
        # Use DNS challenge for wildcard certificates
        dnsProvider = "route53"; # Update this to your DNS provider if different
        environmentFile = "/var/lib/acme/route53-credentials";
      };
    };
  };

  networking.firewall.allowedTCPPorts = [ 80 443 ];

  services.tailscale = {
    enable = true;
    authKeyFile = "/tsauthkey";
    extraUpFlags = [ "--ssh" "--hostname" "flakery-load-balancer" ];
  };

  systemd.services.rp = {
    description = "reverse proxy";
    after = [ "network.target" ];
    wantedBy = [ "multi-user.target" ];
    serviceConfig = {
      ExecStart = "${app}/bin/app";
      Restart = "always";
      KillMode = "process";
    };
  };
}
