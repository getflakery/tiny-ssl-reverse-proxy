{
  description = "A basic gomod2nix flake";

  inputs.nixpkgs.url = "github:NixOS/nixpkgs/nixos-unstable";
  inputs.flake-utils.url = "github:numtide/flake-utils";
  inputs.gomod2nix.url = "github:nix-community/gomod2nix";
  inputs.gomod2nix.inputs.nixpkgs.follows = "nixpkgs";
  inputs.gomod2nix.inputs.flake-utils.follows = "flake-utils";
  inputs.flakery.url = "github:getflakery/flakes";
  inputs.comin.url = "github:r33drichards/comin/cca7a52e63b2d4fc81ec75df902e4f1c25a3048e";



  outputs = inputs@{ self, nixpkgs, flake-utils, gomod2nix, flakery, comin, ... }:
    (flake-utils.lib.eachDefaultSystem
      (system:
        let
          pkgs = nixpkgs.legacyPackages.${system};

          # The current default sdk for macOS fails to compile go projects, so we use a newer one for now.
          # This has no effect on other platforms.
          callPackage = pkgs.darwin.apple_sdk_11_0.callPackage or pkgs.callPackage;

          app = callPackage ./. {
            inherit (gomod2nix.legacyPackages.${system}) buildGoApplication;
          };

        in
        {
          packages.default = app;
          devShells.default = callPackage ./shell.nix {
            inherit (gomod2nix.legacyPackages.${system}) mkGoEnv gomod2nix;
          };

          packages.nixosConfigurations.flakery = nixpkgs.lib.nixosSystem {
            system = system;
            specialArgs = {
              inherit inputs;
            };
            modules = [
              inputs.comin.nixosModules.comin
              flakery.nixosModules.flakery
              {
                security.acme = {
                  acceptTerms = true;
                  defaults.email = "rwendt1337@gmail.com";
                  certs = {
                    "flakery.xyz" = {
                      domain = "*.flakery.xyz";
                      # Use DNS challenge for wildcard certificates
                      dnsProvider = "route53"; # Update this to your DNS provider if different
                      environmentFile = "/var/lib/acme/route53-credentials";
                    };
                  };
                };

                networking.firewall.allowedTCPPorts = [ 80 443 9002 ];

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
                    ExecStart = "${app}/bin/tiny-ssl-reverse-proxy";
                    Restart = "always";
                    KillMode = "process";
                  };
                };
                services.promtail = {
                  enable = true;
                  configuration = {
                    server = {
                      http_listen_port = 9080;
                      grpc_listen_port = 0;
                    };
                    clients = [{ url = "http://grafana:3100/loki/api/v1/push"; }];
                    scrape_configs = [
                      {
                        job_name = "system";
                        static_configs = [
                          {
                            targets = [ "localhost" ];
                            labels = {
                              job = "varlogs";
                              __path__ = "/var/log/*log";
                            };
                          }

                        ];
                      }
                      {
                        job_name = "journal";
                        journal = {
                          max_age = "12h";
                          labels = {
                            job = "systemd-journal";
                            host = "load-balancer";
                          };
                        };
                        relabel_configs = [{
                          source_labels = [ "__journal__systemd_unit" ];
                          target_label = "unit";
                        }];
                      }

                    ];
                  };
                };
                services.prometheus = {
                  enable = true;
                  port = 9090;
                  exporters = {
                    node = {
                      enable = true;
                      enabledCollectors = [ "systemd" ];
                      port = 9002;
                    };

                  };
                };


                services.comin = {
                  enable = true;
                  hostname = "flakery";
                  remotes = [
                    {
                      name = "origin";
                      url = "https://github.com/getflakery/tiny-ssl-reverse-proxy";
                      poller.period = 2;
                      branches.main.name = "master";
                    }
                  ];
                };
              }
            ];
          };

        })
    );
}
