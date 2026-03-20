{
  description = "NixOS SD image for Raspberry Pi 4B — solar IoT monitoring stack";

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
          };

          # ── Users ──────────────────────────────────────────────────
          users.users.monitor = {
            isNormalUser = true;
            extraGroups = [ "wheel" ];
          };

          security.sudo.wheelNeedsPassword = false;

          # ── SSH ────────────────────────────────────────────────────
          services.openssh = {
            enable = true;
            settings = {
              PasswordAuthentication = false;
              PermitRootLogin = "no";
            };
          };

          # ── Mosquitto (MQTT broker) ────────────────────────────────
          services.mosquitto = {
            enable = true;
            listeners = [
              {
                port = 1883;
                settings.allow_anonymous = false;
                acl = [ "topic readwrite #" ];
                users.monitor = {
                  passwordFile = "/var/lib/mosquitto/monitor-password";
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
              install -m 0755 ${pkgs.victoriametrics}/bin/victoria-metrics $out/bin/
            '';
          };

          # ── Vector (MQTT → VictoriaMetrics bridge) ──────────────────
          services.vector = {
            enable = true;
            # Config uses env-var interpolation (${MQTT_PASSWORD}), which
            # is not available during nix-build-time validation.
            validateConfig = false;
            settings = {
              sources.mqtt = {
                type = "mqtt";
                host = "127.0.0.1";
                port = 1883;
                username = "monitor";
                password = "\${MQTT_PASSWORD}";
                topic = "#";
              };

              transforms.to_influx = {
                type = "remap";
                inputs = [ "mqtt" ];
                source = ''
                  topic_raw = string!(.topic)
                  value = to_float!(strip_whitespace(string!(.message)))

                  # solar/pv/voltage → measurement=solar_pv, field=voltage
                  parts = split(topic_raw, "/")
                  if length(parts) < 3 { abort }

                  field = join!(slice!(parts, -1), "")
                  measurement = join!(slice!(parts, 0, -1), "_")

                  .message = measurement + ",mqtt_topic=" + topic_raw + " " + field + "=" + to_string(value)
                '';
              };

              sinks.victoriametrics = {
                type = "http";
                inputs = [ "to_influx" ];
                uri = "http://127.0.0.1:8428/write";
                method = "post";
                encoding.codec = "text";
                healthcheck.enabled = false;
              };
            };
          };

          # Vector must wait for Mosquitto and VictoriaMetrics
          systemd.services.vector = {
            after = [ "mosquitto.service" "victoriametrics.service" ];
            wants = [ "mosquitto.service" "victoriametrics.service" ];
            serviceConfig.EnvironmentFile = "/var/lib/vector/mqtt.env";
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
          time.timeZone = "UTC";
          i18n.defaultLocale = "en_US.UTF-8";
          i18n.supportedLocales = [ "en_US.UTF-8/UTF-8" ];

          nix.settings.experimental-features = [ "nix-command" "flakes" ];
          nix.registry = lib.mkForce {};
          nix.channel.enable = false;

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
  };
}
