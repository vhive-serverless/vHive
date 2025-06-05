#!/bin/bash
# cleanup-runner-space.sh
# Frees up disk space on GitHub Actions runners by removing large preinstalled packages and directories.

set -euo pipefail

echo "=============================================================================="
echo "Freeing up disk space on GitHub Actions runner 🧹"
echo "=============================================================================="

echo ""
echo "Initial disk usage:"
df -h

echo ""
echo "Removing large unused packages..."

# Remove specific packages by pattern (ignore errors gracefully)
remove_patterns=(
  '^ghc-8.*'
  '^dotnet-.*'
  '^llvm-.*'
  'php.*'
)

for pattern in "${remove_patterns[@]}"; do
  echo "  - Removing packages matching: $pattern"
  sudo apt-get remove -y "$pattern" || true
done

# Remove specific large individual packages
large_packages=(
  azure-cli
  google-cloud-sdk
  hhvm
  google-chrome-stable
  firefox
  powershell
  mono-devel
)

echo "  - Removing known large packages..."
sudo apt-get remove -y "${large_packages[@]}" || true

echo ""
echo "Cleaning up unused dependencies and package cache..."
sudo apt-get autoremove -y
sudo apt-get clean

echo ""
echo "Removing large directories..."

large_dirs=(
  /usr/share/dotnet/
  /opt/ghc
  /usr/local/lib/android
)

for dir in "${large_dirs[@]}"; do
  echo "  - Deleting: $dir"
  sudo rm -rf "$dir"
done

echo ""
echo "✅ Disk usage after cleanup:"
df -h
