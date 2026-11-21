#!/usr/bin/env bash
# Download Firecracker + guest kernel and build the base rootfs (Ubuntu base +
# paddock-agent) into the paddock assets dir. No root required.
set -euo pipefail

FC_VERSION=${FC_VERSION:-v1.18.0}
KERNEL_KEY=${KERNEL_KEY:-firecracker-ci/v1.15/x86_64/vmlinux-6.1.155}
UBUNTU_TARBALL=${UBUNTU_TARBALL:-ubuntu-base-24.04.5-base-amd64.tar.gz}
ROOTFS_SIZE=${ROOTFS_SIZE:-4G}

repo=$(cd "$(dirname "$0")/.." && pwd)
data=${PADDOCK_HOME:-${XDG_DATA_HOME:-$HOME/.local/share}/paddock}
assets=$data/assets
cache=$repo/build/cache
work=$repo/build/work
mkdir -p "$assets" "$cache" "$work"

fetch() { # url dest
  if [[ ! -f $2 ]]; then
    echo "fetch $1"
    curl -fL --progress-bar -o "$2.part" "$1"
    mv "$2.part" "$2"
  fi
}

# Firecracker
fc_tgz=$cache/firecracker-$FC_VERSION-x86_64.tgz
fetch "https://github.com/firecracker-microvm/firecracker/releases/download/$FC_VERSION/firecracker-$FC_VERSION-x86_64.tgz" "$fc_tgz"
tar -xzf "$fc_tgz" -C "$work"
install -m 0755 "$work/release-$FC_VERSION-x86_64/firecracker-$FC_VERSION-x86_64" "$assets/firecracker"

# Kernel
kernel=$cache/$(basename "$KERNEL_KEY")
fetch "https://s3.amazonaws.com/spec.ccfc.min/$KERNEL_KEY" "$kernel"
install -m 0644 "$kernel" "$assets/vmlinux"

# Guest agent (static)
echo "build paddock-agent"
(cd "$repo" && CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build -trimpath -ldflags='-s -w' -o "$work/paddock-agent" ./cmd/paddock-agent)

# Rootfs: ubuntu base tarball + agent, written straight into ext4 via
# `mke2fs -d <tarball>` so file ownership is preserved without root.
ubuntu=$cache/$UBUNTU_TARBALL
fetch "https://cdimage.ubuntu.com/ubuntu-base/releases/24.04/release/$UBUNTU_TARBALL" "$ubuntu"
echo "build rootfs.ext4 ($ROOTFS_SIZE)"
gzip -dc "$ubuntu" > "$work/rootfs.tar"
mkdir -p "$work/overlay/usr/sbin" "$work/overlay/etc"
install -m 0755 "$work/paddock-agent" "$work/overlay/usr/sbin/paddock-agent"
echo paddock > "$work/overlay/etc/hostname"
tar -rf "$work/rootfs.tar" -C "$work/overlay" --owner=0 --group=0 --numeric-owner \
  ./usr/sbin/paddock-agent ./etc/hostname
rm -f "$work/rootfs.ext4"
mke2fs -q -t ext4 -L rootfs -d "$work/rootfs.tar" "$work/rootfs.ext4" "$ROOTFS_SIZE"
mv "$work/rootfs.ext4" "$assets/rootfs.ext4"
rm -f "$work/rootfs.tar"

echo "assets ready in $assets"
ls -lh "$assets"
