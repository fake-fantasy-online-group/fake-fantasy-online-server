#!/usr/bin/env bash
set -euo pipefail

script_dir="$(cd "$(dirname "$0")" && pwd)"
repo_root="$(cd "$script_dir/.." && pwd)"

cd "$script_dir"
go build -trimpath -ldflags "-s -w -X main.builtRepoRoot=${repo_root}" -o "$repo_root/fantasy-controller" .

# Keep the launcher next to the binary; no system-wide installation is needed.
# Desktop Entry strings consume one escape layer before Exec is parsed.
desktop_exec_escape() {
    local value="$1"
    value="${value//\\/\\\\}"
    value="${value//\"/\\\"}"
    value="${value//\$/\\\$}"
    value="${value//\`/\\\`}"
    value="${value//\\/\\\\}"
    printf '%s' "$value"
}
icon_path="${script_dir}/assets/controller.png"
icon_path="${icon_path//\\/\\\\}"
cat > "$repo_root/服务端控制器.desktop" <<EOF
[Desktop Entry]
Type=Application
Name=Fantasy服务端控制器
Comment=Fantasy服务端控制器
Exec="$(desktop_exec_escape "$repo_root/fantasy-controller")"
Icon=$icon_path
Terminal=false
Categories=Development;Utility;
StartupWMClass=org.fakefantasy.server-controller
EOF
chmod +x "$repo_root/服务端控制器.desktop"
echo "$repo_root/fantasy-controller"
