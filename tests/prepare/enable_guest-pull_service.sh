#!/bin/bash
set -e

echo "Enabling guest-pull service"
sudo cp tests/config/guest-pull-snapshotter.service /etc/systemd/system/
sudo systemctl daemon-reload
sudo systemctl enable guest-pull-snapshotter
sudo systemctl start guest-pull-snapshotter
sudo systemctl status guest-pull-snapshotter
echo "Guest-pull service enabled"
