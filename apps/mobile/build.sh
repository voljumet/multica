#!/bin/sh
# Build the iOS app (production) and upload to TestFlight.
# Requires: Xcode signed into an Apple ID on the Multica team (automatic signing).
set -eu
cd "$(dirname "$0")"

# Production env + identity (mirrors the ios:prod package.json scripts).
set -a
. ./.env.production
[ -f ./.env.production.local ] && . ./.env.production.local
set +a
export APP_ENV=production

# Version bump: show current version/build, ask for the new version.
# Same version → build number +1; new version → build number resets to 1.
# Rewrites the greppable `version:` / `buildNumber:` lines in app.config.ts.
CUR_VERSION=$(sed -n 's/.*version: "\([^"]*\)".*/\1/p' app.config.ts)
CUR_BUILD=$(sed -n 's/.*buildNumber: "\([^"]*\)".*/\1/p' app.config.ts)
echo "Current version: $CUR_VERSION (build $CUR_BUILD)"
printf 'New version [%s]: ' "$CUR_VERSION"
read -r NEW_VERSION
NEW_VERSION=${NEW_VERSION:-$CUR_VERSION}
if [ "$NEW_VERSION" = "$CUR_VERSION" ]; then
  NEW_BUILD=$((CUR_BUILD + 1))
else
  NEW_BUILD=1
fi
sed -i '' "s/version: \"$CUR_VERSION\"/version: \"$NEW_VERSION\"/" app.config.ts
sed -i '' "s/buildNumber: \"$CUR_BUILD\"/buildNumber: \"$NEW_BUILD\"/" app.config.ts
echo "Building $NEW_VERSION (build $NEW_BUILD)"

# Regenerate the native project so the prod bundle id / display name are baked in
# (ios/ may have last been prebuilt as the dev variant).
pnpm exec expo prebuild -p ios --no-install
(cd ios && pod install)

xcodebuild archive \
  -workspace ios/Multica.xcworkspace \
  -scheme Multica \
  -configuration Release \
  -destination 'generic/platform=iOS' \
  -archivePath build/Multica.xcarchive \
  -allowProvisioningUpdates

# destination=upload sends the build to App Store Connect / TestFlight directly.
xcodebuild -exportArchive \
  -archivePath build/Multica.xcarchive \
  -exportOptionsPlist ExportOptions.plist \
  -exportPath build \
  -allowProvisioningUpdates

echo "Uploaded. Processing takes ~10min — check App Store Connect > TestFlight."
