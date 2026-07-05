#!/bin/bash
# Tags and drafts the GitHub Release for an arm64 build already produced by
# rebuild-deb.sh -- the "manual arm64 flow" ARCHITECTURE.md's release-
# versioning section refers to, since there's no CI runner for the physical
# arm64 dev box the way build-deb.yml covers x64 automatically.
#
# Automates every mechanical part of that flow so it never has to be
# reverse-engineered from a previous release again:
#   - version/tag naming (including the ~ -> - substitution git tag names
#     require, and the ~ -> . substitution GitHub applies to uploaded
#     asset filenames)
#   - finding this build's actual .deb/.buildinfo/.changes files
#   - the "Supersedes vX" / "Same codebase as vX plus:" boilerplate,
#     linked to whichever tag is actually most recent
#   - the required "## arm64 build" section header and one-command-per-
#     code-block install instructions
#   - creating the annotated tag, pushing branch + tag, and creating the
#     release with all of the above attached
#
# Deliberately does NOT try to auto-write the bulleted "what changed" list
# -- that needs real understanding of each fix, not a mechanical diff. By
# default it drafts that section from CHANGELOG.md's entries newer than the
# previous tag and opens $EDITOR on it before the release is created, so the
# content always gets a real read-through. Pass --notes-file=PATH instead to
# skip both the auto-draft and $EDITOR entirely and use PATH's content
# as-is -- for when the notes are being written by someone/something else
# ahead of time rather than edited interactively here.
#
# The release is created as a draft by default -- review the notes, then
# either edit them on GitHub directly or re-run with --publish once
# they're right. Add --summary "..." for the tag message's own short
# parenthetical (matching the existing v6.0.0-beta1-N tag convention).
#
# Usage:
#   scripts/create-arm64-release.sh --summary "short description of what's new"
#   scripts/create-arm64-release.sh --summary "..." --publish
#   scripts/create-arm64-release.sh --summary "..." --dry-run
#   scripts/create-arm64-release.sh --summary "..." --notes-file=/path/to/notes.md

set -euo pipefail

SUMMARY=""
PUBLISH=0
DRY_RUN=0
NOTES_FILE_ARG=""
for arg in "$@"; do
  case "$arg" in
    --summary=*)
      SUMMARY="${arg#--summary=}"
      ;;
    --publish)
      PUBLISH=1
      ;;
    --dry-run)
      DRY_RUN=1
      ;;
    --notes-file=*)
      NOTES_FILE_ARG="${arg#--notes-file=}"
      ;;
    *)
      echo "Unknown argument: $arg" >&2
      echo "Usage: $0 --summary=\"...\" [--publish] [--dry-run] [--notes-file=PATH]" >&2
      exit 1
      ;;
  esac
done

if [[ -n "$NOTES_FILE_ARG" && ! -f "$NOTES_FILE_ARG" ]]; then
  echo "error: --notes-file=$NOTES_FILE_ARG does not exist" >&2
  exit 1
fi

if [[ -z "$SUMMARY" ]]; then
  echo "error: --summary=\"...\" is required (the tag message's short parenthetical)" >&2
  exit 1
fi

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
REPO_ROOT="$(cd "$SCRIPT_DIR/.." && pwd)"
PARENT_DIR="$(cd "$REPO_ROOT/.." && pwd)"
cd "$REPO_ROOT"

REPO_SLUG="anjeleno/rivolution"

REV="$(grep -oP '(?<=@VERSION@-)[0-9]+(?=\))' debian/changelog.src | head -1)"
UPSTREAM_VERSION="$(cat versions/PACKAGE_VERSION)"
PKG_VERSION="${UPSTREAM_VERSION}-${REV}"          # e.g. 6.0.0~beta1-6
TAG="v${PKG_VERSION//\~/-}"                        # e.g. v6.0.0-beta1-6
DOT_VERSION="${PKG_VERSION//\~/.}"                 # e.g. 6.0.0.beta1-6 (GitHub asset form)

BRANCH="$(git rev-parse --abbrev-ref HEAD)"
BRANCH_LABEL="${BRANCH##*/}"
SHORT_SHA="$(git rev-parse --short=8 HEAD)"

if git rev-parse "$TAG" >/dev/null 2>&1; then
  echo "error: tag $TAG already exists -- did you mean to bump the revision first?" >&2
  exit 1
fi

# rebuild-deb.sh deliberately leaves its revision bump uncommitted (commit
# it yourself once happy). Tagging HEAD before that commit exists means the
# tag points at a commit whose debian/changelog.src still declares the OLD
# revision -- confirmed live 2026-07-05 on v6.0.0-beta1-6: the arm64 .deb
# built fine from the uncommitted bump, but CI's from-tag x64 rebuild
# faithfully reproduced the stale committed version, shipping x64 assets
# mislabeled a revision behind. Refuse rather than repeat that.
if ! git diff --quiet -- debian/changelog.src debian/control.src \
   || ! git diff --cached --quiet -- debian/changelog.src debian/control.src; then
  echo "error: debian/changelog.src and/or debian/control.src have uncommitted changes." >&2
  echo "       Commit the revision bump first (it's what rebuild-deb.sh just staged" >&2
  echo "       for you), then re-run this script -- otherwise the tag will point at" >&2
  echo "       a commit that still declares the old revision, and any from-tag" >&2
  echo "       rebuild (e.g. build-deb.yml's x64 CI) will silently ship the wrong" >&2
  echo "       version string." >&2
  exit 1
fi

PREV_TAG="$(git tag --sort=-creatordate | head -1)"
if [[ -z "$PREV_TAG" ]]; then
  echo "error: no existing tags found to supersede -- this script assumes at least one prior release" >&2
  exit 1
fi
PREV_TAG_DATE="$(git log -1 --format=%ad --date=format:%Y-%m-%d "$PREV_TAG")"

echo "==> ${PKG_VERSION} (tag ${TAG}), superseding ${PREV_TAG} (${PREV_TAG_DATE})"

echo "==> Finding built assets in $PARENT_DIR"
mapfile -t ASSETS < <(find "$PARENT_DIR" -maxdepth 1 -type f \
  \( -name "rivolution*_${PKG_VERSION}_*.deb" \
     -o -name "rivolution_${PKG_VERSION}_*.buildinfo" \
     -o -name "rivolution_${PKG_VERSION}_*.changes" \) \
  ! -name "*-dbgsym*" | sort)

if [[ ${#ASSETS[@]} -eq 0 ]]; then
  echo "error: no built assets found for ${PKG_VERSION} in ${PARENT_DIR} -- run rebuild-deb.sh first" >&2
  exit 1
fi
printf '    %s\n' "${ASSETS[@]##*/}"

CLEANUP_NOTES_FILE=0
if [[ -n "$NOTES_FILE_ARG" ]]; then
  echo "==> Using supplied release notes: $NOTES_FILE_ARG"
  NOTES_FILE="$NOTES_FILE_ARG"
else
  echo "==> Drafting release notes from CHANGELOG.md entries newer than ${PREV_TAG_DATE}"
  NOTES_FILE="$(mktemp --suffix=-release-notes.md)"
  CLEANUP_NOTES_FILE=1
  {
    echo "Built from the not-yet-merged"
    echo "[\`${BRANCH}\`](https://github.com/${REPO_SLUG}/tree/${BRANCH})"
    echo "branch (\`${SHORT_SHA}\`), not \`main\` -- a pre-merge test build so this"
    echo "branch's changes can be installed and exercised on a real box before"
    echo "merging, same as every other real-install-tested revision this fork"
    echo "has gone through. Supersedes [${PREV_TAG}](https://github.com/${REPO_SLUG}/releases/tag/${PREV_TAG})"
    echo "with the fixes below. Supersedes nothing on \`main\`; if testing finds"
    echo "something else that needs fixing, expect this release to be replaced"
    echo "too before the branch actually merges."
    echo
    echo "Same codebase as [${PREV_TAG}](https://github.com/${REPO_SLUG}/releases/tag/${PREV_TAG})"
    echo "plus:"
    echo
    echo "<!-- DRAFT from CHANGELOG.md -- rewrite as real release-note prose"
    echo "     before publishing, same voice as previous releases. Entries"
    echo "     below are every CHANGELOG.md bullet dated after ${PREV_TAG_DATE};"
    echo "     trim anything that isn't actually new since ${PREV_TAG}. -->"
    awk -v cutoff="$PREV_TAG_DATE" '
      /^## [0-9]{4}-[0-9]{2}-[0-9]{2}/ { d = substr($0, 4); keep = (d > cutoff); next }
      keep { print }
    ' CHANGELOG.md
    echo
    echo "This is a real test of the ${BRANCH_LABEL} branch -- expect to find more."
    echo
    echo "## arm64 build"
    echo
    echo '```'
    echo "wget https://github.com/${REPO_SLUG}/releases/download/${TAG}/rivolution_${DOT_VERSION}_arm64.deb"
    echo '```'
    echo
    echo '```'
    echo "sudo apt install ./rivolution_${DOT_VERSION}_arm64.deb"
    echo '```'
  } > "$NOTES_FILE"
fi

echo "==> Notes file: $NOTES_FILE"
if [[ "$DRY_RUN" -eq 1 ]]; then
  echo "==> --dry-run: not tagging, pushing, or creating anything. Notes preview:"
  echo "---"
  cat "$NOTES_FILE"
  [[ "$CLEANUP_NOTES_FILE" -eq 1 ]] && rm -f "$NOTES_FILE"
  exit 0
fi

if [[ -z "$NOTES_FILE_ARG" ]]; then
  echo "==> Review/edit the draft now (the CHANGELOG-derived section needs rewriting into prose)."
  "${EDITOR:-nano}" "$NOTES_FILE"
fi

echo "==> Creating annotated tag ${TAG}"
git tag -a "$TAG" -m "${TAG} -- ${BRANCH_LABEL} pre-merge test build (${SUMMARY})"

echo "==> Pushing branch and tag"
git push origin "$BRANCH"
git push origin "$TAG"

DRAFT_FLAG="--draft"
if [[ "$PUBLISH" -eq 1 ]]; then
  DRAFT_FLAG=""
fi

echo "==> Creating GitHub release (${TAG})"
# shellcheck disable=SC2086
gh release create "$TAG" "${ASSETS[@]}" \
  --repo "$REPO_SLUG" \
  --title "$TAG" \
  --notes-file "$NOTES_FILE" \
  $DRAFT_FLAG

[[ "$CLEANUP_NOTES_FILE" -eq 1 ]] && rm -f "$NOTES_FILE"

if [[ "$PUBLISH" -eq 0 ]]; then
  echo "==> Created as a draft. Publish once reviewed:"
  echo "      gh release edit ${TAG} --repo ${REPO_SLUG} --draft=false"
fi
