#!/bin/bash
set -e

git clone https://github.com/ChengyuZhu6/kata-containers.git -b test
cd kata-containers/src/runtime
make
cp containerd-shim-kata-v2 /opt/kata/bin/

