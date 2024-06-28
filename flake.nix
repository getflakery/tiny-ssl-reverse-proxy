{
  description = "A basic gomod2nix flake";

  inputs.nixpkgs.url = "github:NixOS/nixpkgs/nixos-unstable";
  inputs.flake-utils.url = "github:numtide/flake-utils";
  inputs.gomod2nix.url = "github:nix-community/gomod2nix";
  inputs.gomod2nix.inputs.nixpkgs.follows = "nixpkgs";
  inputs.gomod2nix.inputs.flake-utils.follows = "flake-utils";
  inputs.flakery.url = "github:getflakery/flakes";


  outputs = inputs@{ self, nixpkgs, flake-utils, gomod2nix, flakery }:
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
            modules = [
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
            ];
          };

        })
    );
}
