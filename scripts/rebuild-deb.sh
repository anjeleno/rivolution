#!/bin/bash
# Prepares a fully clean tree and builds the .deb packages from scratch:
#   1. Bumps the Debian revision (the "-N" suffix on @VERSION@) by one in
#      both debian/changelog.src and debian/control.src's matching
#      version-pinned Depends lines, so every build here is a genuinely
#      new, distinctly-tagged version -- not a same-numbered rebuild that
#      would collide with whatever was already released under that tag.
#      Leaves the bumped .src files uncommitted; commit them yourself
#      once you're happy with the build.
#   2. Removes stray built package files (.deb/.buildinfo/.changes/.ddeb)
#      left over from a previous build, in the directory dpkg-buildpackage
#      drops them in (one level above the repo root).
#   3. Cleans debian/'s own build-tree artifacts (debhelper stamps/cache,
#      per-package staging directories, .substvars, debian/files,
#      debian/tmp) and regenerates the derived debian/control,
#      debian/rules, and debian/changelog files from their .src templates
#      -- the same sed substitution autogen.sh does for just those three
#      files, without autogen.sh's heavier libtoolize/aclocal/automake/
#      autoconf regeneration, which this doesn't need. Skipping this
#      regeneration step is exactly how a real edit to e.g.
#      debian/control.src silently doesn't make it into a build: the
#      generated file dpkg-buildpackage actually reads just sits stale.
#   4. Runs dpkg-buildpackage itself, parallelized across all cores.
#
# Run from anywhere: cd ~/dev/rivolution && scripts/rebuild-deb.sh
#
# Pass --no-bump to skip step 1 and build the revision already
# committed in debian/changelog.src/control.src as-is -- used by CI,
# which builds whatever revision a tag already points at rather than
# minting a new one.

set -euo pipefail

BUMP_REVISION=1
if [[ "${1:-}" == "--no-bump" ]]; then
  BUMP_REVISION=0
fi

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
REPO_ROOT="$(cd "$SCRIPT_DIR/.." && pwd)"
PARENT_DIR="$(cd "$REPO_ROOT/.." && pwd)"

cd "$REPO_ROOT"

if [[ "$BUMP_REVISION" -eq 1 ]]; then
  CURRENT_REV=$(grep -oP '(?<=@VERSION@-)[0-9]+(?=\))' debian/changelog.src | head -1)
  NEW_REV=$((CURRENT_REV + 1))
  echo "==> Bumping Debian revision: ${CURRENT_REV} -> ${NEW_REV}"
  sed -i "s/@VERSION@-${CURRENT_REV}/@VERSION@-${NEW_REV}/g" debian/changelog.src debian/control.src
else
  echo "==> --no-bump: building revision already committed in debian/*.src"
fi

echo "==> Removing stray built package files in $PARENT_DIR"
find "$PARENT_DIR" -maxdepth 1 -type f \
  \( -name "rivolution*.deb" -o -name "rivolution*.ddeb" \
     -o -name "rivolution*.buildinfo" -o -name "rivolution*.changes" \) \
  -print -delete

echo "==> Cleaning debian/ build-tree artifacts"
rm -rf debian/.debhelper debian/autoreconf.before debian/autoreconf.after \
       debian/files debian/tmp debian/*.substvars
for pkg in $(grep '^Package:' debian/control.src | awk '{print $2}'); do
  rm -rf "debian/$pkg"
done

echo "==> Regenerating debian/control, debian/rules, debian/changelog from their .src templates"
HPKLINUX_DEP=""
if test -f /usr/include/asihpi/hpi.h; then
  HPKLINUX_DEP="\,hpklinux-dev"
fi
sed s/@VERSION@/"$(cat versions/PACKAGE_VERSION)"/ < debian/control.src > debian/control
sed "s/@HPKLINUX_DEP@/$HPKLINUX_DEP/" < debian/control.src > debian/control.src2
sed s/@VERSION@/"$(cat versions/PACKAGE_VERSION)"/ < debian/control.src2 > debian/control
rm -f debian/control.src2
DATESTAMP="$(date +%a,\ %d\ %b\ %Y\ %T\ %z)"
sed s/@VERSION@/"$(cat versions/PACKAGE_VERSION)"/ < debian/changelog.src \
  | sed "s/@DATESTAMP@/$DATESTAMP/" > debian/changelog
sed s/@PYTHONAPI_VERSION@/"$(cat versions/PYTHONAPI_VERSION)"/ < debian/rules.src > debian/rules
chmod +x debian/rules

echo "==> Building (DEBUILD_MAKE_ARGS=\"-j$(nproc)\")"
DEBUILD_MAKE_ARGS="-j$(nproc)" dpkg-buildpackage -us -uc -b

echo "==> Done. Packages in $PARENT_DIR:"
ls -la "$PARENT_DIR"/rivolution*.deb
