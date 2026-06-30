# 0005 — Go API foundation

**Date:** 2026-06-21

## Goal

Establish the architectural foundation for a new Go-based REST API and
web dashboard covering administration functionality currently only
reachable through `RDAdmin`, `RDLogManager`, and `RDCatch`'s native
Qt GUIs. The native playout/library applications (`RDAirplay`,
`RDLibrary`, `RDLogEdit`, `RDPanel`) are explicitly out of scope and
stay native — this spec covers the administration surface only.

## Background

This fork already exposes a comprehensive HTTP API: `rdxport.cgi`
(`web/rdxport/rdxport.cpp`, dispatch at lines 154-387) handles 46
commands — cart/cut CRUD, log management, scheduling, podcasts/RSS,
audio import/export — enumerated in `lib/rdxport_interface.h` and
documented in `docs/apis/web_api.xml`. The existing `apis/rivwebcapi/`
(C) and `apis/rivwebpyapi/` (Python) libraries are thin client wrappers
around this same XML API.

Separately, RML (`lib/rdmacro.cpp`, `ripcd/ripcd.cpp`) is a UDP-based,
internal-only app-to-app control protocol (e.g. `rdairplay`/`rdcatchd`
play/stop/cue commands). It is not a general-purpose API and is out of
scope for this work — the new Go API never touches it.

`rdxport.cgi` ticket-based authentication is bound to the calling IP
address: `RDUser::ticketIsValid()` (`lib/rduser.cpp:716-728`) includes
an `IPV4_ADDRESS` match in its validation query, and
`RDFormPost::authenticate()` (`lib/rdformpost.cpp:354`) calls it with
the request's own client address. `rdxport.cgi` is also a fresh CGI
process per request (`Xport`'s constructor, `web/rdxport/rdxport.cpp`,
reopens the database connection each invocation), not a persistent
service. Both facts shape the auth design below.

`RDUser`/`USER_PERMS` already encodes fine-grained, per-action
permission booleans (`adminConfig`, `createCarts`, `deleteCarts`,
`modifyCarts`, `editAudio`, `createLog`, `playoutLog`, `editCatches`,
`addPodcast`, etc.) plus per-group cart permissions. Any new
authorization layer must read this existing model, not invent a
parallel one.

## Design

### Language choice

Go, not C++. Reusing `RDCart`/`RDUser`/`RDGroup` directly from `lib/`
in C++ would eliminate the native/proxy duplication-risk discussed
below, but would pull this layer back into the Qt/autotools build it
is explicitly meant to be decoupled from, and C++'s JSON/REST/JWT
tooling is materially more manual than Go's. Go's independent
advantages: single static binary (consistent with this project's
multi-arch ARM64/AMD64 deployment target), mature REST/JSON/JWT
ecosystem, and a clean build/deploy separation from the legacy C++/Qt
toolchain.

### Default posture: proxy, not reimplementation

The Go API is primarily a translator/proxy in front of `rdxport.cgi`.
It does not reimplement business logic by default — that has to be
earned per-endpoint. No endpoint talks to RML; RML stays exclusively
internal to the existing native apps.

### Three-bucket risk classification

Every one of the 46 `rdxport.cgi` commands falls into one of three
buckets before any Go code is written for it:

1. **Proxy-only, permanently:** `Export`, `Import`, `ExportPeaks`,
   `TrimAudio`, `CopyAudio`, `AudioInfo`, `AudioStore`, `DeleteAudio` —
   anything touching audio file I/O, format conversion, or `/var/snd`
   directly. This code has a recent, real bug-fix history (MP3
   gain-patch normalization, passthrough sample-rate mismatches,
   Dropbox normalization handling, signed-peak energy bugs).
   Reimplementing it in Go is out of scope for this effort, not a
   "later" item.
2. **Proxy now, native candidate later, case-by-case:** cart/cut CRUD,
   log management, scheduling, podcasts/RSS. Real business rules, no
   audio codec involvement. Migrate to native Go only when a specific
   endpoint has a measured reason (e.g. a latency requirement XML
   parsing can't meet).
3. **Native-Go-first:** pure read-only reference-data listings with no
   write path and no audio/business logic — `ListGroups`, `ListGroup`,
   `ListServices`, `ListSchedCodes`, `ListSystemSettings`. The
   underlying SQL is a handful of joins directly portable from
   `web/rdxport/groups.cpp`/`services.cpp`.

The Go service is built with an internal interface boundary (e.g. a
`CartStore`/`GroupStore`/`LogStore`-style interface per resource type)
so a proxy implementation and a native-MariaDB implementation are
interchangeable — moving a bucket-2 endpoint from proxy to native later
must not require any HTTP handler or JSON schema change.

Native-Go SQL must stay close enough to its C++ source to audit by
inspection — each native query should cite the C++ file/line it
mirrors in a comment, since permission-filtering logic
(`USER_PERMS` joins) duplicated into a second language has to be kept
in lockstep manually.

### Authentication

JWT is a browser↔Go-API session token only — it is never passed to or
validated against `rdxport.cgi`. The Go API's login endpoint accepts a
username/password, calls `RDXPORT_COMMAND_CREATETICKET` against
`rdxport.cgi` on the user's behalf, and on success issues its own JWT
to the browser. The rdxport ticket is held server-side, keyed by JWT
subject — never embedded in the token itself.

Because tickets are IP-bound, the Go service must be `rdxport.cgi`'s
one consistent caller per user: it creates and reuses each user's
ticket from its own fixed IP, rather than forwarding end-user browser
IPs through to `rdxport.cgi`. The trust relationship between the Go
service's host and `rdxport.cgi` should use the `STATIONS`-table trust
path (a row identifying the Go service's host as trusted), not the
`127.0.0.0/8` implicit-loopback path — both work when the two run on
the same host, but `STATIONS` generalizes correctly if the Go service
ever runs on a separate host or in a container, where the loopback
path would silently stop applying.

No independent Go-native user database. Identity and authorization
stay entirely sourced from the existing `USERS`/`USER_PERMS` tables for
the foreseeable future — a fully independent Go-native auth system is
only a sensible end state once enough user-management functionality
has itself been ported to native Go, which is not in scope here.

### Container-forward design

Every component built under this spec should be designed so that
containerizing it later doesn't require a rewrite — concretely, no
hardcoded same-host assumptions baked into request routing or service
discovery, even though containerized deployment isn't implemented now.

## Approach: Phase 1

The first real, buildable, testable slice covers exactly one native
endpoint and one proxy endpoint, deliberately together:

- `GET /api/v1/groups` — native Go, reimplementing
  `web/rdxport/groups.cpp`'s query directly against MariaDB. No write
  path — smallest possible blast radius for the first Go code in this
  project.
- `GET /api/v1/carts` and `GET /api/v1/carts/{number}` — proxy mode,
  translating to `RDXPORT_COMMAND_LISTCARTS`/`RDXPORT_COMMAND_LISTCART`
  against `rdxport.cgi`, parsing the XML response, re-serializing as
  JSON.

Phase 1's purpose is proving the hybrid pattern end-to-end — auth
passthrough, proxy plumbing, native DB access, JSON contract, error
handling — with zero write risk, not merely confirming a Go server can
run. Phase 2 (not covered by this spec) is expected to add the first
write paths once both read paths are proven.

## Files

- New: `rivapi/` at the repository root — a top-level directory
  parallel to `rdadmin/`, `rdairplay/`, and the other application
  directories, following the same flat structure and the `riv` prefix
  convention established for new v6 components. Go module path:
  `github.com/anjeleno/rivolution/rivapi`. Binary name: `rivapi`.
- Reference only, unmodified by this spec: `web/rdxport/rdxport.cpp`,
  `web/rdxport/groups.cpp`, `lib/rdxport_interface.h`, `lib/rduser.cpp`,
  `lib/rdformpost.cpp`, `docs/apis/web_api.xml`.

## Verification

1. `GET /api/v1/groups` output compared field-for-field against
   `rdxport.cgi`'s `ListGroups` XML response for the same authenticated
   user, to catch permission-filtering divergence between the native
   query and the original immediately.
2. `GET /api/v1/carts`/`GET /api/v1/carts/{number}` verified against a
   real `rdxport.cgi` instance with a known seeded library.
3. Ticket lifecycle verified end-to-end: login issues a JWT, the Go
   service's cached rdxport ticket is reused across multiple requests
   without re-authenticating against `rdxport.cgi` each time, and
   ticket expiry is handled by re-issuing rather than failing silently.

## Implementation deviations

None yet — implementation has not started.
