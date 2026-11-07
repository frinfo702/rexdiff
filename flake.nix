{
  description = "A diff viewer for coding agents in Rex";

  inputs.nixpkgs.url = "github:NixOS/nixpkgs/nixpkgs-unstable";

  outputs =
    { self, nixpkgs }:
    let
      systems = [
        "aarch64-darwin"
        "x86_64-darwin"
        "aarch64-linux"
        "x86_64-linux"
      ];
      forAll = f: nixpkgs.lib.genAttrs systems (s: f nixpkgs.legacyPackages.${s});
    in
    {
      packages = forAll (pkgs: {
        default = pkgs.buildGo127Module {
          pname = "rexdiff";
          version = self.shortRev or self.dirtyShortRev or "dev";
          src = self;
          vendorHash = "sha256-7bU/Nr4WwIW8RZWqEC5RnUdCGqAF3/nVB5KeumkAacM=";
          env.CGO_ENABLED = 0;
          ldflags = [ "-s" "-w" ];
          nativeCheckInputs = [ pkgs.git ];
          meta.mainProgram = "rexdiff";
        };
      });
    };
}
