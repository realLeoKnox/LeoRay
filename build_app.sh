#!/bin/bash
set -e

APP_VERSION="1.0.2"
BUILD_VERSION="1.2"

DIR="$( cd "$( dirname "${BASH_SOURCE[0]}" )" && pwd )"
PROJECT="$DIR"
APP="$PROJECT/LeoRay.app"
RES="$APP/Contents/Resources"

echo "━━━━━━━━━━━━━━━━━━━━━━━━━━━━"
echo "  LeoRay Build Script"
echo "━━━━━━━━━━━━━━━━━━━━━━━━━━━━"

# ── 1. Compile Go backend ───────────────────────────────────────────────────
echo "▶ [1/3] 编译 Go 后端..."
cd "$PROJECT/go"
go build -o "$PROJECT/xray_controller" .
echo "   ✓ xray_controller"

# ── 2. Compile Swift UI ─────────────────────────────────────────────────────
echo "▶ [2/3] 编译 Swift UI..."
cd "$PROJECT/LeoRayUI"
swift build -c release 2>&1
SWIFT_BIN="$PROJECT/LeoRayUI/.build/release/LeoRay"
echo "   ✓ LeoRay (Swift binary)"

# ── 3. Assemble .app bundle ─────────────────────────────────────────────────
echo "▶ [3/3] 组装 LeoRay.app..."
rm -rf "$APP"
mkdir -p "$APP/Contents/MacOS"
mkdir -p "$RES/core" "$RES/data" "$RES/config" "$RES/static"

# Main binary
cp "$SWIFT_BIN" "$APP/Contents/MacOS/LeoRay"
chmod +x "$APP/Contents/MacOS/LeoRay"

# Go backend + assets
cp "$PROJECT/xray_controller"            "$RES/"
cp "$PROJECT/core/xray"                  "$RES/core/"
cp "$PROJECT/data/geoip.dat"             "$RES/data/"
cp "$PROJECT/data/geosite.dat"           "$RES/data/"
# Also copy geo files to core/ — xray's default lookup dir when XRAY_LOCATION_ASSET is unset
# (this handles TUN/sudo scenarios where sudo strips the env var)
cp "$PROJECT/data/geoip.dat"             "$RES/core/"
cp "$PROJECT/data/geosite.dat"           "$RES/core/"
cp "$PROJECT/data/custom_nodes.json"     "$RES/data/"
cp "$PROJECT/data/sub"                   "$RES/data/"
if [ -f "$PROJECT/config/policy.json" ]; then
    cp "$PROJECT/config/policy.json"     "$RES/config/"
fi
cp "$PROJECT/config/default_policy.md"   "$RES/config/"
cp "$PROJECT/static/index.html"          "$RES/static/"
cp "$PROJECT/LeoRayUI/LeoRay.icns"       "$RES/LeoRay.icns"

# Ensure binaries are executable
chmod +x "$RES/xray_controller"
chmod +x "$RES/core/xray"

# Info.plist
cat > "$APP/Contents/Info.plist" << PLIST
<?xml version="1.0" encoding="UTF-8"?>
<!DOCTYPE plist PUBLIC "-//Apple//DTD PLIST 1.0//EN"
  "http://www.apple.com/DTDs/PropertyList-1.0.dtd">
<plist version="1.0">
<dict>
    <key>CFBundleIconFile</key>         <string>LeoRay.icns</string>
    <key>CFBundleName</key>             <string>LeoRay</string>
    <key>CFBundleDisplayName</key>      <string>LeoRay</string>
    <key>CFBundleIdentifier</key>       <string>de.leoknox.leoray</string>
    <key>CFBundleVersion</key>          <string>${BUILD_VERSION}</string>
    <key>CFBundleShortVersionString</key> <string>${APP_VERSION}</string>
    <key>CFBundleExecutable</key>       <string>LeoRay</string>
    <key>CFBundlePackageType</key>      <string>APPL</string>
    <key>LSMinimumSystemVersion</key>   <string>13.0</string>
    <key>LSUIElement</key>              <true/>
    <key>NSSupportsAutomaticGraphicsSwitching</key> <true/>
</dict>
</plist>
PLIST

echo ""
echo "✅ 完成！"
echo "   应用路径: $APP"
echo ""
echo "   首次运行如被 Gatekeeper 拦截，请执行:"
echo "   xattr -cr \"$APP\""
echo ""
echo "   启动命令:"
echo "   open \"$APP\""
