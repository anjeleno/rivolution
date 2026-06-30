# Spec 0013 — RDAdmin parity roadmap

## Purpose

This spec is a classification and sequencing document, not a per-screen
design doc. Its job is to map all ~48 RDAdmin management sections
against the Go dashboard's implementation strategy, so each subsequent
dashboard PR has a clear place in the order and knows which
implementation bucket it belongs to before design begins.

The three-bucket model from spec 0005 (originally used to classify
rdxport.cgi's 46 commands) is extended here to cover all dashboard
sections, including those for which rdxport.cgi has no coverage at all.

## Implementation buckets

### Bucket A — rdxport proxy, full CRUD already available

rdxport.cgi provides read + write commands covering these entities.
Dashboard implementation: `CartProxy`/`LogProxy` (same pattern as the
existing `CartProxy` in `rivapi/store/carts_proxy.go`).

| Section | rdxport commands |
|---------|-----------------|
| Carts | 6 (list), 7 (get), 12 (add), 14 (edit), 13 (remove) |
| Cuts | 9 (list), 8 (get), 10 (add), 15 (edit), 11 (remove) |
| Audio import/export | 2 (import), 1 (export), 3 (delete), 16–19, 23 |
| Logs | 20 (list), 22 (get), 29 (add), 28 (save), 30 (delete), 34 (lock) |
| Podcasting/Feeds (publish) | 37–46 (get/save/delete/post/remove podcast, RSS, images) |
| Schedule code cart-assignment | 25 (assign), 26 (unassign), 27 (list on cart) |

### Bucket B — rdxport proxy, read-only

rdxport.cgi exposes a read-only view; write operations require native
Go+DB (Bucket C or D). Dashboard shows the read view today; the write
path is added when that section's PR is picked up.

| Section | rdxport read command | Write path |
|---------|---------------------|-----------|
| Groups (list/get) | 4, 5 | Bucket C |
| Services (list) | 21 | Bucket D |
| System Settings (read) | 33 | Bucket D |
| Schedule code list | 24 | Bucket C (CRUD) |

### Bucket C — native Go+DB, pattern established

These sections have no rdxport write commands (or no commands at all),
but the implementation pattern already exists: mirror the C++ source's
exact SQL/field semantics into a new `*DB` store type, same as
`GroupDB` in `rivapi/store/groups_db.go` mirrors `lib/rdgroup.cpp`.

| Section | C++ source reference | DB tables |
|---------|---------------------|-----------|
| Groups CRUD | `rdadmin/edit_group.cpp`, `lib/rdgroup.cpp` | GROUPS |
| Users + user permissions | `rdadmin/edit_user.cpp`, `rdadmin/edit_user_perms.cpp`, `lib/rduser.cpp` | USERS, USER_PERMS |
| User service permissions | `rdadmin/edit_user_service_perms.cpp` | SERVICE_PERMS |
| Schedule code CRUD | `rdadmin/edit_schedcodes.cpp` | SCHED_CODES |
| Feeds (definition, perms, superfeed) | `rdadmin/edit_feed.cpp`, `rdadmin/edit_feed_perms.cpp`, `rdadmin/edit_superfeed.cpp` | FEEDS, FEED_PERMS, FEED_METADATA |

### Bucket D — native Go+DB, larger surface, spec-per-section

These sections have zero rdxport coverage and represent new, non-trivial
store work. Each gets its own design spec when its PR is starting — they
are not designed here in advance.

| Section | Files | DB tables |
|---------|-------|-----------|
| Hosts/Stations | `rdadmin/edit_station.cpp`, `rdadmin/list_stations.cpp` | STATIONS |
| — Dropboxes | `rdadmin/edit_dropbox.cpp` | DROPBOXES |
| — Decks | `rdadmin/edit_decks.cpp` | DECKS, DECK_CHANNELS |
| — Cart Slots | `rdadmin/edit_cartslots.cpp` | CART_SLOTS |
| — Audio Ports | `rdadmin/edit_audios.cpp` | AUDIO_INPUTS, AUDIO_OUTPUTS |
| — TTY Devices | `rdadmin/edit_ttys.cpp` | TTYS |
| — Switchers/Matrices | `rdadmin/edit_matrix.cpp` | MATRICES, MATRIX_PINS |
| — GPI Inputs | `rdadmin/edit_gpi.cpp` | GPIS |
| — Livewire/AES67 GPIOs | `rdadmin/edit_livewiregpio.cpp` | LIVEWIRE_GPIOS |
| — Livewire Nodes | `rdadmin/edit_node.cpp` | LIVEWIRE_NODES |
| — PyPAD Instances | `rdadmin/edit_pypad.cpp` | PYPADS |
| — Host Variables | `rdadmin/edit_hostvar.cpp` | HOSTVARS |
| — vGuest Resources | `rdadmin/edit_vguest_resource.cpp` | VGUEST_RESOURCES |
| — SAS Resources | `rdadmin/edit_sas_resource.cpp` | SAS_RESOURCES |
| — RDAirPlay config | `rdadmin/edit_rdairplay.cpp` | RDAIRPLAY, RDAIRPLAY_CARTS |
| — RDLibrary config | `rdadmin/edit_rdlibrary.cpp` | RDLIBRARY |
| — RDLogedit config | `rdadmin/edit_rdlogedit.cpp` | RDLOGEDIT |
| — RDPanel config | `rdadmin/edit_rdpanel.cpp` | RDPANEL |
| — JACK config | `rdadmin/edit_jack.cpp` | JACK, JACK_CLIENTS |
| — Endpoints | `rdadmin/edit_endpoint.cpp` | ENDPOINTS |
| — Channel GPIOs | `rdadmin/edit_channelgpios.cpp` | CHANNEL_GPIOS |
| Services CRUD | `rdadmin/edit_svc.cpp` | SERVICES |
| System Settings (write) | `rdadmin/edit_system.cpp` | SYSTEM |
| Reports | `rdadmin/edit_report.cpp` | REPORTS |
| Replicators | `rdadmin/edit_replicator.cpp` | REPLICATORS, REPLICATOR_CARTS |
| Images | `rdadmin/edit_image.cpp` | IMAGES |
| Autofill Carts | `rdadmin/autofill_carts.cpp` | (utility, no dedicated table) |
| Station Branding | (new, no RDAdmin equivalent) | SYSTEM or new table |

Note: the Hosts/Stations section and its 15 sub-dialogs are the largest
single Bucket-D item. It is also the prerequisite for spec 0010's goal
of the dashboard being the sole interface for systemd configuration
changes — the host config objects (CAE settings, audio ports, dropboxes)
are what spec 0010's "zero-disruption / deferred / explicit" config
change categories operate on.

## Build order

Priority is determined by: (1) operational importance — what breaks or
is unmanageable without it; (2) dependency order — Bucket C before D;
(3) connection to other in-flight specs.

1. **Dashboard shell** (spec 0012, current PR) — auth, template layout,
   Groups/Carts/Logs read views.
2. **Groups write path** (Bucket C) — extends the already-built
   `GroupDB`; lowest-risk first write-path PR.
3. **Users + permissions** (Bucket C) — needed early; permissions gate
   what the logged-in user can see throughout the dashboard.
4. **Carts/Cuts CRUD + audio import/export** (Bucket A) — the most
   content-management-heavy section; the existing `CartProxy` gets
   write-method companions.
5. **Logs CRUD** (Bucket A) — log editing is core to daily operations.
6. **Hosts/Stations** (Bucket D, starts with its own spec) —
   prerequisite for spec 0010 dashboard-as-orchestrator goal. Largest
   single section; likely multiple stacked PRs internally.
7. **Services CRUD + System Settings write** (Bucket D).
8. **Schedule codes CRUD + cart-assignment UI** (Bucket C + A).
9. **Feeds/Podcasting full management** (Bucket A/C).
10. **Reports, Replicators, Images, Autofill** (Bucket D long tail).
11. **Station Branding editor** (Bucket D, Rivolution-exclusive).

## Rivolution-exclusive sections

The following have no RDAdmin equivalent and are tracked here rather
than in the parity sections above:

- **Network / Tailscale** — installer role, auth-key activation,
  MagicDNS and TLS cert provisioning. Spec 0014.
- **Station Branding** — logo upload, colors, station name; persisted
  to DB. Slot exists in `base.html` (spec 0012) from day one;
  the editor comes later.
- **Systemd stack control** — service start/stop/restart, readiness
  status display. Specified in spec 0010; dashboard implementation is
  the Hosts/Stations PR's companion.
