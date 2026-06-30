# 0011 — Database schema migration strategy

**Date:** 2026-06-30

## Goal

Define how this fork manages its own database schema changes
independently of upstream Rivendell's `RD_VERSION_DATABASE` counter,
without colliding with any future upstream version increments, while
preserving the ability to bring a v4 Rivendell database forward into
v6 cleanly and to cherry-pick schema-neutral changes back upstream
without interference.

## Background

Rivendell manages schema migrations through a single integer field
`DB_VERSION` in the `SYSTEM` table (confirmed present in
`utils/rddbmgr/create.cpp`), exposed as `RD_VERSION_DATABASE` in
`lib/rd.h`. The upgrade path lives in `utils/rddbmgr/updateschema.cpp`:
a sequence of conditional blocks, each keyed to the current value of
`DB_VERSION`, incrementing it after applying each migration.

This fork diverges substantially enough from upstream that it will
accumulate its own schema changes that upstream will never have —
`docs/specs/0007-pipewire-audio-engine.md`'s new `AUDIO_CARDS` columns
for AES67 config being the first confirmed example. Two problems arise
if v6 simply keeps incrementing `DB_VERSION` in upstream's sequence:

1. Any future upstream release that also increments `DB_VERSION` (for
   its own migration) silently collides — same version number, different
   schema, no way to distinguish which codebase applied the migration.
2. `DB_VERSION` loses its meaning as an identifier: it no longer
   unambiguously describes what state a database is in, because the same
   value could mean "upstream v4.x at this version" or "v6 applied a
   fork-specific migration at this number."

The fork is already diverged far enough that upstream compatibility at
the code level is not a design target. Schema compatibility at the
upgrade path level — being able to bring a real operator's existing v4
database into v6 without data loss — is a target, and is what this spec
is designed to preserve.

## Design

### `RIVOLUTION_DB_VERSION` counter

A new integer column `RIVOLUTION_DB_VERSION` is added to the existing
`SYSTEM` table. This is the sole counter for v6 schema changes.
`DB_VERSION` (the upstream counter) is left entirely unmodified by v6:
not incremented, not reset, not renamed. It retains the value that the
last upstream-compatible migration left it at.

Concretely:

- **Fresh v6 install:** `rivolution-first-run.sh`'s database creation
  sets `DB_VERSION` to the schema version that `create.cpp` currently
  establishes (whatever upstream value v6's `create.cpp` last touched),
  and sets `RIVOLUTION_DB_VERSION` to the current v6 schema version
  (the count of v6 migrations that have been applied in `create.cpp`
  for a fresh install).
- **v4 database upgraded to v6:** `rddbmgr` first applies any pending
  upstream-schema migrations (keyed on `DB_VERSION`) that the database
  hasn't yet seen, then applies v6 migrations (keyed on
  `RIVOLUTION_DB_VERSION`, starting from 0 if the column doesn't exist
  yet). The result is a database with both counters at their correct
  post-migration values.
- **Subsequent v6 migrations:** keyed only on `RIVOLUTION_DB_VERSION`.
  `DB_VERSION` is never touched again once a database is under v6.

### Migration convention

Each v6 schema migration consists of:

1. A conditional block in `utils/rddbmgr/updateschema.cpp` keyed to the
   current value of `RIVOLUTION_DB_VERSION`, applying the schema change
   and incrementing the counter.
2. A corresponding entry in `utils/rddbmgr/create.cpp` applying the
   same change for fresh installs (so a new install doesn't have to
   replay all migrations from zero).
3. A `CHANGELOG.md` entry citing the migration number and what changed.

Migrations are numbered starting at 1, incrementing by 1 with no gaps.
Migration 0 is the implicit "no v6 migrations applied yet" state —
`RIVOLUTION_DB_VERSION` absent or 0 — and requires no code, just the
column-add to `SYSTEM` that initializes it.

### `rddbmgr` invocation order

`rddbmgr`'s upgrade flow applies upstream migrations first, v6 migrations
second. This ensures a v4 database reaches a fully upstream-migrated
state before v6-specific columns or tables are added on top of it —
avoiding any ordering conflict where a v6 migration assumes a column that
the upstream migration path hasn't added yet.

### Upstream compatibility

Schema-neutral changes to this fork (logic changes in `updateschema.cpp`
blocks that don't add or remove columns) can still be cherry-picked back
to upstream `ElvishArtisan/rivendell` without touching either version
counter. Schema changes that add new columns or tables are
fork-specific by definition and are not candidates for upstream
contribution, but the counter separation means they don't interfere with
upstream's own counter even if both projects are active.

## Files

- Modified: `utils/rddbmgr/create.cpp` — add `RIVOLUTION_DB_VERSION
  INTEGER NOT NULL DEFAULT 0` column to the `SYSTEM` table definition,
  set to the correct current v6 schema version for fresh installs.
- Modified: `utils/rddbmgr/updateschema.cpp` — add the initial migration
  block that adds `RIVOLUTION_DB_VERSION` to existing databases (checks
  for column absence, adds it with value 0, does not increment anything
  — this is the bootstrapping step that makes subsequent v6 migrations
  possible on a database that originated before this spec landed).
- `lib/rddbmgr.h` / `utils/rddbmgr/rddbmgr.cpp` — expose
  `RIVOLUTION_DB_VERSION` through whatever accessor the existing code
  uses for `DB_VERSION`, so callers can read the v6 version without
  raw SQL.

## Verification

1. Fresh install: confirm `RIVOLUTION_DB_VERSION` column exists in
   `SYSTEM` after `rivolution-first-run.sh`, `DB_VERSION` holds the
   expected upstream value, and both read back correctly through
   `rddbmgr`.
2. Upgrade path: take a real v4-schema database (e.g., one of the
   existing seed SQL files at the repo root), run `rddbmgr --check` then
   `rddbmgr --update`, confirm both counters end at their expected
   post-migration values with no errors.
3. Add a test migration (v6 migration #1, the AES67 columns from spec
   0007) against the same upgraded v4 database and confirm it applies
   cleanly on top of the upstream migration results.

## Implementation deviations

None yet — implementation has not started.
