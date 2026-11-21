{
  description = "paddock: self-hosted microVM sandboxes for AI agents";

  inputs.nixpkgs.url = "github:NixOS/nixpkgs/nixos-unstable";

  outputs =
    { nixpkgs, ... }:
    let
      systems = [
        "x86_64-linux"
        "aarch64-linux"
      ];
      forAllSystems = f: nixpkgs.lib.genAttrs systems (system: f nixpkgs.legacyPackages.${system});
    in
    {
      devShells = forAllSystems (pkgs: {
        default = pkgs.mkShell {
          packages = with pkgs; [
            go
            golangci-lint
            gofumpt
            gotools
            gnumake
            e2fsprogs
            curl
            cpio
          ];
          env.CGO_ENABLED = "0";
        };
      });
    };
}
