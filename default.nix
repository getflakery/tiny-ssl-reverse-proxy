{ pkgs ? (
    let
      inherit (builtins) fetchTree fromJSON readFile;
      inherit ((fromJSON (readFile ./flake.lock)).nodes) nixpkgs gomod2nix;
    in
    import (fetchTree nixpkgs.locked) {
      overlays = [
        (import "${fetchTree gomod2nix.locked}/overlay.nix")
      ];
    }
  )
, buildGoApplication ? pkgs.buildGoApplication
, lib
}:

buildGoApplication {
  pname = "myapp";
  version = "0.1";
  pwd = ./.;
  src = pkgs.lib.sources.cleanSourceWith {
    src = ./.;
    filter = name: type:
      let
        baseName = baseNameOf (toString name);
      in
      (
        (pkgs.lib.sources.cleanSourceFilter name type) ||
        # base name ends with .nix
        pkgs.lib.hasSuffix ".nix" baseName ||
        baseName == ".direnv" ||
        baseName == "target" ||
        # has prefix flake 
        pkgs.lib.hasPrefix "flake" baseName ||
        # ends with .lock
        pkgs.lib.hasSuffix ".lock" baseName
        # ends with yml
        pkgs.lib.hasSuffix ".yml" baseName

      );
  };
  modules = ./gomod2nix.toml;
}
