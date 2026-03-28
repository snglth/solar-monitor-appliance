{
  description = "NixOS SD image for Raspberry Pi 4B — solar IoT monitoring stack";

  nixConfig = {
    extra-substituters = [ "https://cache.garnix.io" ];
    extra-trusted-public-keys = [ "cache.garnix.io:CTFPyKSLcx5RMJKfLo5EEPUObbA78b0YQ2DTCJXqr9g=" ];
  };

  inputs = {
    nixpkgs.url = "github:NixOS/nixpkgs/nixos-25.11";
    nixos-hardware.url = "github:NixOS/nixos-hardware";
  };

  outputs = { self, nixpkgs, nixos-hardware }: {
    nixosConfigurations.solar-monitor = nixpkgs.lib.nixosSystem {
      system = "aarch64-linux";
      modules = [
        nixos-hardware.nixosModules.raspberry-pi-4
        "${nixpkgs}/nixos/modules/installer/sd-card/sd-image-aarch64.nix"
        "${nixpkgs}/nixos/modules/profiles/minimal.nix"
        ./modules/user-config.nix

        ({ config, pkgs, lib, ... }: {

          # ── SD image ───────────────────────────────────────────────
          # Disable the built-in compression pipeline to avoid a wasteful
          # compress (rootfs) → decompress → compress (final image) cycle.
          # Instead, compress only the final assembled image via postBuildCommands.
          sdImage.compressImage = false;
          sdImage.postBuildCommands = ''
            ${pkgs.zstd}/bin/zstd -T$NIX_BUILD_CORES --rm $img
          '';

          # The sd-image module sets noauto on the firmware partition,
          # but apply-user-config needs it mounted to read config.json.
          fileSystems."/boot/firmware".options = lib.mkForce [ "nofail" ];

          # ── Boot / hardware ────────────────────────────────────────
          # nixos-hardware module handles device-tree, firmware, kernel.
          # Allow unfree for Raspberry Pi firmware blobs.
          nixpkgs.config.allowUnfree = true;

          # The sd-image module inherits the installer profile, which
          # enables ZFS, btrfs, cifs, xfs, etc. by default. The Pi only
          # needs ext4 + vfat — strip the rest to shed ~230 MiB (samba,
          # python3, etc.).
          boot.supportedFilesystems = lib.mkForce { ext4 = true; vfat = true; };
          services.lvm.enable = false;
          boot.initrd.systemd.enable = true;
          boot.swraid.enable = lib.mkForce false;

          # sd-image-aarch64.nix sets generic aarch64 initrd modules (incl.
          # dw-hdmi, which was removed/renamed in kernel 6.6+). The RPi
          # kernel doesn't ship all of those. Override with only what the
          # Pi 4 actually needs for early boot.
          boot.initrd.availableKernelModules = lib.mkForce [
            "mmc_block"       # SD card block device layer
            "usbhid"
            "usb_storage"
            "uas"
            "pcie-brcmstb"
            "reset-raspberrypi"
          ];

          boot.initrd.includeDefaultModules = false;

          hardware.enableAllHardware = lib.mkForce false;
          hardware.enableRedistributableFirmware = lib.mkForce false;
          hardware.firmware = [ pkgs.raspberrypiWirelessFirmware ];

          # The nixos-hardware RPi4 module sets a DTB filter that fails
          # in sandboxed builds (fchmodat2/EPERM). Disable filtering —
          # the extra DTBs are negligible on the firmware partition.
          hardware.deviceTree.filter = lib.mkForce null;

          # The RPi vendor kernel doesn't ship all modules that NixOS
          # references. Allow missing modules instead of failing.
          nixpkgs.overlays = [
            (final: prev: {
              makeModulesClosure = args:
                prev.makeModulesClosure (args // { allowMissing = true; });
            })
          ];

          fonts.enableDefaultPackages = false;

          # ── Networking ─────────────────────────────────────────────
          networking.hostName = "solar-monitor";

          networking.useNetworkd = true;
          systemd.network.networks."20-wired" = {
            matchConfig.Name = "end0";
            address = [ "10.44.0.1/24" ];
            networkConfig = {
              DHCPServer = true;
              DHCP = "no";
            };
            dhcpServerConfig = {
              PoolOffset = 100;
              PoolSize = 100;
              EmitDNS = false;
              EmitRouter = false;
            };
          };

          networking.wireless.iwd = {
            enable = true;
            settings = {
              Settings.AutoConnect = true;
            };
          };
          systemd.network.networks."30-wireless" = {
            matchConfig.Name = "wlan0";
            networkConfig.DHCP = "yes";
          };

          networking.firewall = {
            enable = true;
            allowedTCPPorts = [
              1883  # Mosquitto MQTT
              3000  # Grafana
            ];
            allowedUDPPorts = [ 5353 ];  # mDNS
            trustedInterfaces = [ "end0" ];
          };

          # ── Users ──────────────────────────────────────────────────
          users.users.monitor = {
            isNormalUser = true;
            extraGroups = [ "wheel" ];
            hashedPassword = "$6$uo9M30/dxyG5bpam$s/4zzLDv/egqBjXBXbZVWErqfYZXfxpDOPf4qwe0oReeuRck23ZY27gFOlSoovcbZCNpSH38kG.OQBDk/LS2C0";
          };

          security.sudo.wheelNeedsPassword = false;

          # ── SSH ────────────────────────────────────────────────────
          services.openssh = {
            enable = true;
            settings = {
              PasswordAuthentication = true;
              PermitRootLogin = "no";
            };
          };

          # ── mDNS (Avahi) ─────────────────────────────────────────────
          services.avahi = {
            enable = true;
            nssmdns4 = true;
            publish = {
              enable = true;
              addresses = true;
            };
          };

          # ── Mosquitto (MQTT broker) ────────────────────────────────
          services.mosquitto = {
            enable = true;
            listeners = [
              {
                port = 1883;
                settings.allow_anonymous = false;
                users.monitor = {
                  passwordFile = "/var/lib/mosquitto/monitor-password";
                  acl = [ "readwrite #" ];
                };
              }
            ];
            logType = [ "information" "warning" "error" ];
          };

          # ── VictoriaMetrics ─────────────────────────────────────────
          services.victoriametrics = {
            enable = true;
            retentionPeriod = "12";  # months
            # Default package ships 8 Go binaries (~173 MiB); we only need
            # the single-node server (~16 MiB). Copy just that binary so
            # the others drop out of the image closure without rebuilding.
            package = pkgs.runCommand "victoriametrics-slim" {} ''
              mkdir -p $out/bin
              cp ${pkgs.victoriametrics}/bin/victoria-metrics $out/bin/
              chmod 0755 $out/bin/victoria-metrics
            '';
          };

          # ── MQTT bridge (MQTT → VictoriaMetrics) ────────────────────
          # Lightweight Go binary (~3 MiB) that subscribes to all MQTT
          # topics, reformats as InfluxDB line protocol, and POSTs to
          # VictoriaMetrics /write.
          systemd.services.mqtt-bridge = let
            mqttBridge = pkgs.buildGoModule {
              pname = "mqtt-bridge";
              version = "0.1.0";
              src = self + "/cmd/mqtt-bridge";
              vendorHash = "sha256-Db09ftEG9DJgN6mb4LaA2cOGiOjQx36DzeDqzAik2Fs=";
            };
          in {
            description = "MQTT to VictoriaMetrics bridge";
            wantedBy = [ "multi-user.target" ];
            after = [ "mosquitto.service" "victoriametrics.service" ];
            wants = [ "mosquitto.service" "victoriametrics.service" ];
            serviceConfig = {
              ExecStart = "${mqttBridge}/bin/mqtt-bridge";
              EnvironmentFile = "/run/credentials/mqtt.env";
              Restart = "always";
              RestartSec = 5;
            };
          };

          # ── Grafana ────────────────────────────────────────────────
          services.grafana = {
            enable = true;

            # The stock package is ~591 MiB (295 MiB binary, 318 MiB
            # frontend assets incl. 155 MiB of JS source maps, plus
            # grafana-cli/grafana-server we don't need). Copy only
            # what the service actually uses — no source rebuild.
            package = pkgs.runCommand "grafana-slim" {} ''
              mkdir -p $out/bin $out/share/grafana
              install -m 0755 ${pkgs.grafana}/bin/grafana $out/bin/
              cp -a ${pkgs.grafana}/share/grafana/conf $out/share/grafana/
              cp -r --no-preserve=mode ${pkgs.grafana}/share/grafana/public $out/share/grafana/public

              find $out/share/grafana/public -name '*.js.map' -delete

              # Strip unused built-in plugins (~30 MiB). Dashboard uses only
              # stat + timeseries panels and the prometheus datasource.
              cd $out/share/grafana/public/app/plugins
              find panel -mindepth 1 -maxdepth 1 \
                ! -name timeseries ! -name stat ! -name gauge ! -name table ! -name text \
                -exec rm -rf {} +
              find datasource -mindepth 1 -maxdepth 1 \
                ! -name prometheus ! -name grafana ! -name dashboard ! -name mixed \
                -exec rm -rf {} +

              # Strip non-English locales (~10 MiB) and geo data
              find $out/share/grafana/public/locales -mindepth 1 -maxdepth 1 \
                ! -name en-US -exec rm -rf {} +
              rm -rf $out/share/grafana/public/gazetteer
              rm -rf $out/share/grafana/public/maps
            '';

            # Disable plugin installer/updater; appropriate for an appliance
            declarativePlugins = [];

            settings = {
              server = {
                http_addr = "0.0.0.0";
                http_port = 3000;
              };
              security.secret_key = "$__file{/var/lib/grafana/secret_key}";
            };

            provision = {
              enable = true;
              datasources.settings.datasources = [
                {
                  name = "VictoriaMetrics";
                  uid = "victoriametrics";
                  type = "prometheus";
                  url = "http://127.0.0.1:8428";
                  isDefault = true;
                }
              ];
              dashboards.settings.providers = [
                {
                  name = "default";
                  options.path = ./dashboards;
                }
              ];
            };
          };

          systemd.tmpfiles.rules = [
            "d /var/lib/grafana 0750 grafana grafana -"
          ];

          # Generate Grafana secret key on first boot
          systemd.services.grafana-secret-key = {
            description = "Generate Grafana secret key";
            wantedBy = [ "grafana.service" ];
            before = [ "grafana.service" ];
            unitConfig.ConditionPathExists = "!/var/lib/grafana/secret_key";
            serviceConfig = {
              Type = "oneshot";
              ExecStart = pkgs.writeShellScript "gen-grafana-key" ''
                ${pkgs.openssl}/bin/openssl rand -hex 32 > /var/lib/grafana/secret_key
                chmod 400 /var/lib/grafana/secret_key
                chown grafana:grafana /var/lib/grafana/secret_key
              '';
            };
          };

          # ── System basics ──────────────────────────────────────────
          services.getty.helpLine = ''

            Wired:    10.44.0.1 (end0)
            Wireless: \4{wlan0}
            Hostname: solar-monitor.local

            Grafana:  http://solar-monitor.local:3000  (admin / admin)
            SSH:      ssh monitor@solar-monitor.local
            MQTT:     solar-monitor.local:1883  user: monitor
          '';

          time.timeZone = "Europe/Kyiv";
          i18n.defaultLocale = "en_US.UTF-8";
          i18n.supportedLocales = [ "en_US.UTF-8/UTF-8" ];

          # Appliance — no Nix commands needed at runtime.
          # Saves ~50–150 MiB (nix binary, Boehm GC, SQLite, libcurl).
          nix.enable = false;

          documentation.enable = false;

          # Disable installer tools that pull in Python3 (~110 MiB) and Perl (~59 MiB)
          system.tools = {
            nixos-rebuild.enable = false;
            nixos-generate-config.enable = false;
            nixos-install.enable = false;
            nixos-enter.enable = false;
            nixos-option.enable = false;
          };

          environment.defaultPackages = lib.mkForce [];

          environment.systemPackages = with pkgs; [
            htop
            curl
            mosquitto   # CLI tools: mosquitto_pub, mosquitto_sub
          ];

          system.stateVersion = "25.11";
        })
      ];
    };

    packages.aarch64-linux.default =
      self.nixosConfigurations.solar-monitor.config.system.build.sdImage;

    devShells = nixpkgs.lib.genAttrs [ "x86_64-linux" "aarch64-linux" "aarch64-darwin" "x86_64-darwin" ] (system:
      let pkgs = nixpkgs.legacyPackages.${system}; in {
        default = pkgs.mkShellNoCC {
          packages = [
            pkgs.mosquitto
            (pkgs.python3.withPackages (p: [ p.paho-mqtt ]))
            pkgs.platformio-core
          ];
        };
      }
    );
  };
}
