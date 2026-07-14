#!/usr/bin/env bash
set -euo pipefail

ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
APP_NAME="Argos UI Runner"
BUNDLE_ID="ai.meshclaw.argosrunner"
VERSION="${VERSION:-0.1.0}"
BUILD_NUMBER="${BUILD_NUMBER:-1}"
GOARCH_VALUE="${GOARCH:-$(go env GOARCH)}"
OUT_DIR="${ROOT}/dist"
APP_DIR="${OUT_DIR}/${APP_NAME}.app"
MACOS_DIR="${APP_DIR}/Contents/MacOS"
RESOURCES_DIR="${APP_DIR}/Contents/Resources"
INSTALL="${INSTALL:-0}"
INSTALL_DIR="${INSTALL_DIR:-${HOME}/Applications}"
REPLACE_EXISTING="${ARGOS_RUNNER_REPLACE:-0}"
SIGN_IDENTITY="${SIGN_IDENTITY:--}"

rm -rf "${APP_DIR}"
mkdir -p "${MACOS_DIR}" "${RESOURCES_DIR}"

GOOS=darwin GOARCH="${GOARCH_VALUE}" go build -o "${MACOS_DIR}/argos-ui-runner" "${ROOT}/cmd/argos-ui-runner"

if command -v xcrun >/dev/null 2>&1; then
  xcrun swiftc \
    -O \
    -parse-as-library \
    -target "arm64-apple-macosx15.0" \
    -framework ScreenCaptureKit \
    -framework AVFoundation \
    -framework CoreMedia \
    -framework Foundation \
    -o "${MACOS_DIR}/argos-screen-recorder" \
    "${ROOT}/cmd/argos-ui-runner/screen_recorder.swift"
  xcrun swiftc \
    -O \
    -parse-as-library \
    -target "arm64-apple-macosx13.0" \
    -framework EventKit \
    -framework Foundation \
    -o "${MACOS_DIR}/argos-reminder-helper" \
    "${ROOT}/cmd/argos-ui-runner/reminder_helper.swift"
  xcrun swiftc \
    -O \
    -parse-as-library \
    -target "arm64-apple-macosx13.0" \
    -framework EventKit \
    -framework Foundation \
    -o "${MACOS_DIR}/argos-calendar-helper" \
    "${ROOT}/cmd/argos-ui-runner/calendar_helper.swift"
  xcrun swiftc \
    -O \
    -parse-as-library \
    -target "arm64-apple-macosx13.0" \
    -framework Contacts \
    -framework Foundation \
    -o "${MACOS_DIR}/argos-contacts-helper" \
    "${ROOT}/cmd/argos-ui-runner/contacts_helper.swift"
else
  echo "xcrun is required to build the native macOS helpers" >&2
  exit 1
fi

cat > "${APP_DIR}/Contents/Info.plist" <<PLIST
<?xml version="1.0" encoding="UTF-8"?>
<!DOCTYPE plist PUBLIC "-//Apple//DTD PLIST 1.0//EN"
  "http://www.apple.com/DTDs/PropertyList-1.0.dtd">
<plist version="1.0">
<dict>
  <key>CFBundleDevelopmentRegion</key>
  <string>en</string>
  <key>CFBundleExecutable</key>
  <string>argos-ui-runner</string>
  <key>CFBundleIdentifier</key>
  <string>${BUNDLE_ID}</string>
  <key>CFBundleInfoDictionaryVersion</key>
  <string>6.0</string>
  <key>CFBundleName</key>
  <string>${APP_NAME}</string>
  <key>CFBundlePackageType</key>
  <string>APPL</string>
  <key>CFBundleShortVersionString</key>
  <string>${VERSION}</string>
  <key>CFBundleVersion</key>
  <string>${BUILD_NUMBER}</string>
  <key>LSMinimumSystemVersion</key>
  <string>12.0</string>
  <key>LSUIElement</key>
  <true/>
  <key>NSAppleEventsUsageDescription</key>
  <string>Argos UI Runner controls Signal Desktop only when MeshClaw policy allows a call or delivery action.</string>
  <key>NSScreenCaptureUsageDescription</key>
  <string>Argos UI Runner records short proof videos of approved assistant actions so the user can review what Argos did.</string>
  <key>NSRemindersUsageDescription</key>
  <string>Argos UI Runner creates reminders only after MeshClaw policy and user approval allow a reminder action.</string>
  <key>NSCalendarsUsageDescription</key>
  <string>Argos UI Runner creates calendar events only after MeshClaw policy and user approval allow a calendar action.</string>
  <key>NSContactsUsageDescription</key>
  <string>Argos UI Runner searches contacts only after MeshClaw policy and user approval allow a contacts action.</string>
  <key>NSHumanReadableCopyright</key>
  <string>MeshClaw</string>
</dict>
</plist>
PLIST

if command -v xattr >/dev/null 2>&1; then
  xattr -dr com.apple.quarantine "${APP_DIR}" >/dev/null 2>&1 || true
fi

codesign --force --deep --sign "${SIGN_IDENTITY}" "${APP_DIR}"

if [[ "${INSTALL}" == "1" ]]; then
  TARGET_APP="${INSTALL_DIR}/${APP_NAME}.app"
  mkdir -p "${INSTALL_DIR}"
  if [[ -d "${TARGET_APP}" && "${REPLACE_EXISTING}" != "1" ]]; then
    EXISTING_ID="$(/usr/libexec/PlistBuddy -c 'Print :CFBundleIdentifier' "${TARGET_APP}/Contents/Info.plist" 2>/dev/null || true)"
    if [[ "${EXISTING_ID}" == "${BUNDLE_ID}" ]]; then
      echo "preserved existing stable Runner: ${TARGET_APP}"
      echo "set ARGOS_RUNNER_REPLACE=1 only when you intentionally want macOS to re-evaluate Runner permissions"
      exit 0
    fi
  fi
  if [[ -d "${TARGET_APP}" ]]; then
    mv "${TARGET_APP}" "${TARGET_APP}.prev.$(date +%Y%m%d%H%M%S)"
  fi
  cp -R "${APP_DIR}" "${TARGET_APP}"
  if command -v xattr >/dev/null 2>&1; then
    xattr -dr com.apple.quarantine "${TARGET_APP}" >/dev/null 2>&1 || true
  fi
  codesign --verify --strict --verbose=2 "${TARGET_APP}" >/dev/null
  echo "${TARGET_APP}"
  exit 0
fi

echo "${APP_DIR}"
