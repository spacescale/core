#!/bin/bash

#------------------------------------------ NOTES -------------------------------------------------------------------
# 🛑 DO NOT RUN THIS SCRIPT DIRECTLY ON A SERVER 🛑
# This file is a RAW TEMPLATE. It is NOT the final execution script.his script is embedded as a raw string inside the
# `scalecp` Go control plane.When a new OVH node is purchased, `scalecp` uses Go's fmt.Sprintf() to INJECT dynamic secrets
# (NATS Tokens, Node IDs) directly into the string.`scalecp` then connects via SSH and pipes the dynamically generated script
# into the new server's memory. Once this script finishes, it disables SSH permanently.



# Any dynamic cluster credentials belong in the Go injection layer.




#------------------------------------------ NOTES -------------------------------------------------------------------


# Exit immediately if any command exits with a non-zero status.
# This prevents the script from locking SSH if a download fails.
set -e


# OVH minimal images might lack wget or updated SSL certs. Fix that first.
apt-get update -y
apt-get install -y wget ca-certificates curl



# Create the directory for SpaceScale state and cached runtime assets.
mkdir -p /var/lib/spacescale

# scaled owns runtime asset resolution for firecracker, jailer, kernel, and scoutd.
# The bootstrap script only installs the daemon and starts it.
wget https://spacescale-runtime-assets.s3.eu-west-par.io.cloud.ovh.net/scaled -O /usr/local/bin/scaled
chmod +x /usr/local/bin/scaled

# Create the production systemd service file dynamically
# Use 'tee' to safely write the file, bypassing bash redirection permission issues
tee /etc/systemd/system/scaled.service > /dev/null << 'EOF'
[Unit]
Description=SpaceScale Edge Daemon (scaled)
Documentation=https://github.com/spacescale
After=network-online.target
Wants=network-online.target

[Service]
Type=simple
ExecStart=/usr/local/bin/scaled
Restart=always
RestartSec=3

# Production Resource Limits for Hypervisors
LimitNOFILE=1048576
LimitNPROC=infinity
LimitCORE=infinity

# Security isolation for the daemon itself
NoNewPrivileges=yes

[Install]
WantedBy=multi-user.target
EOF

# Reload systemd, enable the service to survive reboots, and start it
systemctl daemon-reload
systemctl enable scaled
systemctl start scaled

#### Lock down host permanently
systemctl stop ssh
systemctl disable ssh
