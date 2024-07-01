let pkgs = import <nixpkgs> {}; in

pkgs.mkShell {
  packages = with pkgs; [
    go gopls
    libGL
    xorg.libX11
    xorg.libXrandr
    xorg.libXcursor
    xorg.libXinerama
    xorg.libXi
    xorg.libXxf86vm

    graphviz
  ];

  shellHook = ''
    export LD_LIBRARY_PATH=${pkgs.lib.getLib pkgs.libGL}/lib:$LD_LIBRARY_PATH
  '';
}

