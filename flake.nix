{
  description = "A basic gomod2nix flake";

  inputs.nixpkgs.url = "github:NixOS/nixpkgs/24.05";
  inputs.flake-utils.url = "github:numtide/flake-utils";
  inputs.gomod2nix.url = "github:nix-community/gomod2nix";
  inputs.gomod2nix.inputs.nixpkgs.follows = "nixpkgs";
  inputs.gomod2nix.inputs.flake-utils.follows = "flake-utils";
  inputs.flakery.url = "github:getflakery/flakes";
  inputs.comin.url = "github:r33drichards/comin/ac3b4d89a571c33ed201fc656287076d0eadb47f";




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
                    "flakery.dev" = {
                      domain = "*.flakery.dev";
                      # Use DNS challenge for wildcard certificates
                      dnsProvider = "route53"; # Update this to your DNS provider if different
                      environmentFile = "/var/lib/acme/route53-credentials";
                    };
                  };
                };

                networking.firewall.allowedTCPPorts = [ 80 443 9002 3000 ];

                services.tailscale = {
                  enable = true;
                  authKeyFile = "/tsauthkey";
                  extraUpFlags = [ "--ssh" "--hostname" "flakery-load-balancer" ];
                };

                systemd.services.rp = {
                  environment = {
                    "JWT_SECRET" = (pkgs.lib.removeSuffix "\n" (builtins.readFile /jwt-secret));
                    "FLAKERY_API_KEY" = (pkgs.lib.removeSuffix "\n" (builtins.readFile /flakery-api-token));
                  };
                  description = "reverse proxy";
                  after = [ "network.target" "acme-flakery.xyz.service" ];
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

                virtualisation.containers.enable = true;
                virtualisation = {
                  podman = {
                    enable = true;

                    # Create a `docker` alias for podman, to use it as a drop-in replacement
                    dockerCompat = true;
                    # docker socket
                    dockerSocket.enable = true;

                    # Required for containers under podman-compose to be able to talk to each other.
                    defaultNetwork.settings.dns_enabled = true;
                  };
                };
                networking.firewall.interfaces.podman0.allowedUDPPorts = [ 53 ];

                # make nocodb depend on create-dir
                virtualisation.oci-containers.backend = "podman";

                virtualisation.oci-containers.containers = {
                  flakery = {
                    image = "public.ecr.aws/t7q1f7c9/flakery:latest";
                    autoStart = true;
                    ports = [ "3000:3000" ];
                    environmentFiles = [ "/.env" ];
                    extraOptions = [ "--pull=newer" ];
                    # extraOptions = [ "--cap-add=CAP_NET_RAW" ]; # maybe not needed

                  };
                  watchtower = {
                    image = "containrrr/watchtower";
                    autoStart = true;
                    volumes = [ "/var/run/docker.sock:/var/run/docker.sock" ];
                    # -i 2
                    cmd = [ "-i" "2" ];
                    extraOptions = [ "--pull=newer" ];

                  };
                };
                systemd.services.podman-flakery = {
                  serviceConfig = {
                    Restart = "always";
                  };
                };

                systemd.services.assign-eip = {
                  description = "Assign Elastic IP to instance";
                  wantedBy = [ "multi-user.target" ];
                  path = with pkgs; [ awscli2 curl ];
                  environment = {
                    AWS_ACCESS_KEY_ID = (pkgs.lib.removeSuffix "\n" (builtins.readFile /AWS_ACCESS_KEY_ID));
                    AWS_SECRET_ACCESS_KEY = (pkgs.lib.removeSuffix "\n" (builtins.readFile /AWS_SECRET_ACCESS_KEY));
                    AWS_REGION = "us-west-2";
                  };
                  script = ''
                    ELASTIC_IP="44.235.22.226"
                    INSTANCE_ID=$(curl -s http://169.254.169.254/latest/meta-data/instance-id)
                    echo "Instance ID: $INSTANCE_ID"
                    ALLOCATION_ID=$(aws ec2 describe-addresses --public-ips $ELASTIC_IP --query 'Addresses[0].AllocationId' --output text)
                    echo "Allocation ID: $ALLOCATION_ID"
                    ASSOCIATION_ID=$(aws ec2 describe-addresses --public-ips $ELASTIC_IP --query 'Addresses[0].AssociationId' --output text)
                    echo "Association ID: $ASSOCIATION_ID"

                    if [ "$ASSOCIATION_ID" != "None" ]; then
                      echo "Elastic IP is already associated with another instance. Disassociating..."
                      aws ec2 disassociate-address --association-id "$ASSOCIATION_ID"
                    fi

                    echo "Associating Elastic IP with instance..."

                    aws ec2 associate-address --instance-id "$INSTANCE_ID" --allocation-id "$ALLOCATION_ID"

                    
                  '';
                  serviceConfig = {
                    Type = "oneshot";
                    RemainAfterExit = true;
                    RestartSec = 32;
                    Restart = "on-failure";
                  };
                };

              }
            ];
          };

          packages.test = pkgs.testers.runNixOSTest
            {
              name = "test health check";
              nodes = {

                machine1 = { pkgs, ... }: {
                  systemd.services.rp = {
                    environment = {
                      "JWT_SECRET" = "test";
                      "FLAKERY_API_KEY" = "test";
                      "FLAKERY_BASE_URL" = "http://localhost:8080";
                    };
                    description = "reverse proxy";
                    after = [ "network.target" ];
                    wantedBy = [ "multi-user.target" ];
                    serviceConfig = {

                      ExecStart = "${app}/bin/tiny-ssl-reverse-proxy --only-healthcheck ";
                      Restart = "always";
                      KillMode = "process";
                    };
                  };

                  systemd.services.serve = {
                    wantedBy = [ "multi-user.target" ];
                    path = [ pkgs.python3 ];
                    script = "${./serve.py}";
                    serviceConfig = {
                      Restart = "always";
                      RestartSec = 0;
                    };
                  };
                };

                machine2 = { ... }: {
                  networking.firewall.allowedTCPPorts = [ 9002 ];
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

                };
              };
              testScript = ''
                start_all()
                # wait for rp to start 
                machine1.wait_for_unit("rp.service")
                # wait for prometheus to start
                machine2.wait_for_unit("prometheus.service")
                # assert that machine1 determines that machine2 is healthy
                result = machine1.wait_until_succeeds("journalctl -xeu rp.service --no-pager | grep -Eo 'Healthy'")
                result = machine1.wait_until_succeeds("journalctl -xeu rp.service --no-pager | grep -Eo 'Healthy'")
                print(result)
                machine2.crash()
                # assert that machine1 determines that machine2 is unhealthy
                result = machine1.wait_until_succeeds("journalctl -xeu rp.service --no-pager | grep -Eo 'UnHealthy'")
                print(result)
                # Received POST request at {self.path} with body: {post_data.decode('utf-8')} in machine1
                result = machine1.wait_until_succeeds("journalctl -xeu serve.service --no-pager | grep -Eo 'Received POST request at /api/deployments/target/unhealthy/230f97a2-8e84-4d9b-8246-11caf8e4507a with body: {\"Host\":\"http://machine2:8080\"}'")
                print(result)
              '';

            };
        }
      )
    );
}
