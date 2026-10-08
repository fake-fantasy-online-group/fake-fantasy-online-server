#!/bin/zsh
set -euo pipefail

script_dir="${0:A:h}"
repo_root="${script_dir:h}"
app_path="${repo_root}/服务端控制器.app"
build_root="$(mktemp -d "${TMPDIR:-/tmp}/fantasy-controller.XXXXXX")"
trap 'rm -rf "$build_root"' EXIT

cd "$script_dir"
go build -trimpath -ldflags "-s -w -X main.builtRepoRoot=${repo_root}" -o "$build_root/fantasy-controller" .

mkdir -p "$build_root/服务端控制器.app/Contents/MacOS"
mkdir -p "$build_root/服务端控制器.app/Contents/Resources"
cp "$script_dir/assets/controller.icns" "$build_root/服务端控制器.app/Contents/Resources/controller.icns"
cp "$build_root/fantasy-controller" "$build_root/服务端控制器.app/Contents/MacOS/fantasy-controller"
cp "$script_dir/Info.plist" "$build_root/服务端控制器.app/Contents/Info.plist"
/usr/bin/codesign --force --deep --sign - "$build_root/服务端控制器.app"

if [[ -e "$app_path" ]]; then
  if [[ "$app_path" != *.app ]]; then
    echo "拒绝替换非 App 路径: $app_path" >&2
    exit 1
  fi
  rm -rf "$app_path"
fi
mv "$build_root/服务端控制器.app" "$app_path"
echo "$app_path"
