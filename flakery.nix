app:
{ config, lib, pkgs, ... }:
{
  networking.firewall.allowedTCPPorts = [ 80 443 ];

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
