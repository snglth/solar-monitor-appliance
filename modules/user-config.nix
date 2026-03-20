{ config, pkgs, lib, ... }:

let
  defaultConfigAttrs = {
    timezone = "UTC";
    wifi = {
      ssid = "";
      password = "";
    };
    ssh_authorized_keys = [];
    mqtt = {
      password = "";
    };
  };
  defaultConfig = builtins.toJSON defaultConfigAttrs;

  applyUserConfig = pkgs.buildGoModule {
    pname = "apply-user-config";
    version = "0.1.0";
    src = ../cmd/apply-user-config;
    vendorHash = null;
  };
in
{
  # ── Seed config.json onto the firmware partition at build time ──
  sdImage.populateFirmwareCommands = lib.mkAfter ''
    ${pkgs.jq}/bin/jq . ${pkgs.writeText "config.json" defaultConfig} > firmware/config.json
  '';

  # ── Systemd oneshot to apply config at early boot ──────────────
  systemd.services.apply-user-config = {
    description = "Apply user config from /boot/firmware/config.json";
    wantedBy = [ "multi-user.target" ];

    after = [
      "local-fs.target"
      "systemd-tmpfiles-setup.service"
    ];
    before = [
      "iwd.service"
      "sshd.service"
      "mosquitto.service"
      "vector.service"
      "grafana.service"
    ];

    serviceConfig = {
      Type = "oneshot";
      RemainAfterExit = true;
      ExecStart = "${applyUserConfig}/bin/apply-user-config --timedatectl=${pkgs.systemd}/bin/timedatectl";
    };

    # Never block boot, even on failure
    unitConfig.FailureAction = "none";
  };

  # ── Mosquitto data directory ─────────────────────────────────
  systemd.tmpfiles.rules = [
    "d /var/lib/mosquitto 0750 mosquitto mosquitto -"
  ];
}
