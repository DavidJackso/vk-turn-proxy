#!/bin/bash
# ============================================================
# VK-TURN Proxy — APK Build Script (Full Auto)
# One-shot: installs JDK, Android SDK, NDK, builds APK.
# ============================================================

set -e

CYAN='\033[0;36m'
GREEN='\033[0;32m'
RED='\033[0;31m'
NC='\033[0m'

log() { echo -e "${CYAN}>>> $1${NC}"; }
ok()  { echo -e "${GREEN}✔  $1${NC}"; }
err() { echo -e "${RED}✘  $1${NC}"; exit 1; }

SCRIPT_DIR="$(cd "$(dirname "$0")" && pwd)"
SDK_DIR="$HOME/android-sdk"
NDK_VER="27.2.12479018"
BUILD_TOOLS_VER="35.0.0"
PLATFORM_VER="35"
CMDTOOLS_URL="https://dl.google.com/android/repository/commandlinetools-linux-11076708_latest.zip"
APP_ID="com.vkturn.proxy"
APP_NAME="VKTurnProxy"

# ── 1. JDK ───────────────────────────────────────────────────
if ! command -v javac &>/dev/null; then
    log "Installing OpenJDK 17..."
    sudo apt-get update -qq
    sudo apt-get install -y openjdk-17-jdk-headless
    ok "JDK installed."
else
    ok "JDK already installed: $(javac -version 2>&1)"
fi

# ── 2. Fyne CLI ──────────────────────────────────────────────
if ! command -v fyne &>/dev/null; then
    log "Installing fyne CLI..."
    go install fyne.io/fyne/v2/cmd/fyne@latest
fi
ok "fyne CLI: $(fyne version 2>&1 | head -1)"

# ── 3. Android SDK cmdline-tools ─────────────────────────────
SDKMANAGER="$SDK_DIR/cmdline-tools/latest/bin/sdkmanager"
if [ ! -f "$SDKMANAGER" ]; then
    log "Downloading Android cmdline-tools..."
    mkdir -p "$SDK_DIR/cmdline-tools"
    TMP_ZIP=$(mktemp --suffix=.zip)
    wget -q --show-progress "$CMDTOOLS_URL" -O "$TMP_ZIP"
    unzip -q "$TMP_ZIP" -d "$SDK_DIR/cmdline-tools"
    mv "$SDK_DIR/cmdline-tools/cmdline-tools" "$SDK_DIR/cmdline-tools/latest"
    rm "$TMP_ZIP"
    ok "cmdline-tools ready."
else
    ok "cmdline-tools already installed."
fi

export ANDROID_HOME="$SDK_DIR"
export PATH="$PATH:$SDK_DIR/cmdline-tools/latest/bin:$SDK_DIR/platform-tools"

# ── 4. Accept licences ───────────────────────────────────────
log "Accepting Android SDK licenses..."
yes | "$SDKMANAGER" --licenses >/dev/null 2>&1 || true

# ── 5. Install NDK + build-tools + platform ──────────────────
log "Installing NDK $NDK_VER, build-tools $BUILD_TOOLS_VER, platform $PLATFORM_VER..."
"$SDKMANAGER" \
    "ndk;$NDK_VER" \
    "build-tools;$BUILD_TOOLS_VER" \
    "platforms;android-$PLATFORM_VER"
ok "Android SDK components installed."

export ANDROID_NDK_HOME="$SDK_DIR/ndk/$NDK_VER"

# ── 6. Build APK ─────────────────────────────────────────────
log "Building APK (this may take a few minutes)..."
cd "$SCRIPT_DIR"

fyne package \
    -os android \
    -appID "$APP_ID" \
    -name "$APP_NAME"

APK_FILE="$SCRIPT_DIR/${APP_NAME}.apk"
if [ -f "$APK_FILE" ]; then
    ok "APK built successfully: $APK_FILE"
    ls -lh "$APK_FILE"
else
    # fyne might output to a different name
    FOUND=$(find "$SCRIPT_DIR" -name "*.apk" | head -1)
    if [ -n "$FOUND" ]; then
        ok "APK built: $FOUND"
        ls -lh "$FOUND"
    else
        err "APK not found. Check output above for errors."
    fi
fi

echo ""
echo -e "${GREEN}╔══════════════════════════════════════════╗${NC}"
echo -e "${GREEN}║         APK build complete!              ║${NC}"
echo -e "${GREEN}╚══════════════════════════════════════════╝${NC}"
echo ""
echo "  Transfer to Android device:"
echo "  adb install $APK_FILE"
echo "  — or — copy manually via USB/cloud."
echo ""
