# Spec 0014 — Tailscale integration

## Problem

Rivolution installs are typically reached either on a local network or
through a Tailscale tailnet. Tailscale provides encrypted, zero-config
overlay networking, automatic MagicDNS hostnames, and optionally
Let's Encrypt-issued TLS certs — making it the natural fit for "connect
to any Rivolution server, from anywhere on the tailnet, without
configuring a VPN or opening firewall ports."

This is not in RDAdmin. It is a Rivolution-exclusive capability.

## Goals

1. **Installer**: Tailscale is available as an optional package in the
   unified installer, installed but not activated.
2. **Dashboard activation**: an admin pastes a pre-auth key from the
   Tailscale admin console to connect the host to their tailnet,
   entirely from the Rivolution dashboard — no terminal required.
3. **Status visibility**: tailnet connectivity, IP address, MagicDNS
   hostname, and cert status are visible in the dashboard's Network
   section.
4. **TLS cert provisioning**: once HTTPS Certs is enabled tailnet-wide,
   the dashboard can provision a cert via `tailscale cert` and optionally
   point `rivapi`'s listener at the resulting cert+key files.

## Sequencing

This spec is implemented *after* spec 0012's network topology config
(`RIVAPI_TRUST_PROXY_HEADERS`, `RIVAPI_COOKIE_SECURE`) is in place,
since that config is what allows cookie `Secure` flag to be set
correctly once TLS is live.

## Piece 1: installer role

In the `rivolution-unified-installer` repo, add
`roles/tailscale/tasks/main.yml` following the same apt + systemd
pattern used by `roles/broadcast_tools/tasks/main.yml`:

```yaml
- name: Install Tailscale
  ansible.builtin.apt:
    name: tailscale
    state: present

- name: Enable tailscaled (do not start — activation is user-driven)
  ansible.builtin.systemd:
    name: tailscaled
    enabled: true
    state: stopped
```

Gate the role with an opt-in group var `rivolution_tailscale_enabled`
(default: `false`) rather than bundling it unconditionally. `tailscaled`
is enabled (survives reboots) but left stopped — the first `tailscale up`
starts it. The Tailscale apt repository needs to be added as a preceding
task (same approach as Tailscale's official install script, but
expressed as Ansible apt_key + apt_repository tasks for idempotency).

## Piece 2: dashboard — auth-key activation and status

A "Network" section in the dashboard's nav (added in the Hosts/Stations
Bucket-D PR, consistent with that section's scope, or as its own small
PR after the shell lands).

### Required privilege

`rivapi` runs as the `rd` user. Running `tailscale up` and
`tailscale cert` requires root (or at minimum the `rd` user to have
the right). A narrow sudoers rule grants exactly these three commands
and nothing else:

```
rd ALL=(root) NOPASSWD: /usr/bin/tailscale up *, /usr/bin/tailscale cert *, /usr/bin/tailscale status
```

The installer role provisions this rule. No unrestricted sudo. This is
the same privilege-scoping model already decided in spec 0010 for
systemd unit control.

### Activation flow

1. Admin pastes a pre-auth key (generated in the Tailscale admin console
   under Settings → Keys) into the dashboard's "Connect to tailnet" field.
2. Dashboard POST handler runs `tailscale up --authkey=<key>` (via
   `exec.Command`). Returns success or error message.
3. Status panel refreshes via htmx polling or a manual refresh button,
   calling `tailscale status --json`, and displays: connection state,
   tailnet name, tailnet IP, MagicDNS hostname.

Pre-auth keys are single-use (or reusable if the admin configured that).
They are not stored after use — the call to `tailscale up` is a
one-shot operation.

### Key types the admin should use

The spec should document this for users:
- **Reusable, ephemeral** keys: appropriate for single-machine
  installations; expire after the configured duration.
- **One-time** keys: more secure; generate a fresh key for each
  Rivolution host being connected.
- **Tagged keys** with `tag:rivolution` (or similar): recommended for
  organizations managing multiple Rivolution installs — enables
  ACL rules scoped to Rivolution hosts in the tailnet.

## Piece 3: MagicDNS and TLS cert provisioning

### What the dashboard can do

Once connected to the tailnet and `tailscale status` confirms a
MagicDNS hostname, the dashboard shows a "Provision TLS cert" button
that runs:

```
sudo tailscale cert <hostname>.ts.net
```

This writes `<hostname>.ts.net.crt` and `<hostname>.ts.net.key` to the
current directory (or a configurable `RIVAPI_TLS_CERT_DIR`). If
successful, `rivapi` restarts its listener in TLS mode using those
files — or, if `rivapi` is behind a reverse proxy, the dashboard
displays the cert paths and instructs the admin to point the proxy at
them.

A new pair of config env vars for TLS:
- `RIVAPI_TLS_CERT` — path to cert file (enables TLS listener when set)
- `RIVAPI_TLS_KEY` — path to key file

When both are set, `rivapi` calls `http.ListenAndServeTLS` instead of
`http.ListenAndServe`. The `RIVAPI_COOKIE_SECURE` flag should be
`true` in this configuration.

### Hard limits — document these plainly

Enabling HTTPS Certs tailnet-wide is a one-time setting in the
**Tailscale admin console** (Settings → DNS → Enable HTTPS). This cannot
be toggled by the local `tailscale` CLI or by `tailscaled`'s local API.
The dashboard should detect whether HTTPS Certs is available by
attempting `tailscale cert --help` or by checking for the cert endpoint
in `tailscale status --json`, and display a clear "Enable HTTPS Certs
in the Tailscale admin console first" message when it is not available —
rather than silently failing or implying it is possible to enable from
here.

## Dashboard UI sketch

```
Network
  Tailscale status: ● Connected
  Tailnet:          example.ts.net
  Tailnet IP:       100.x.y.z
  Hostname:         rivolution-studio.example.ts.net
  TLS cert:         ✓ Valid, expires 2027-06-30

  [ Reconnect / change key ]  [ Reprovision cert ]
```

When not connected:

```
Network
  Tailscale status: ○ Not connected

  Connect to tailnet
  Auth key: [__________________________]  [ Connect ]
  (Generate a pre-auth key at tailscale.com/admin/settings/keys)
```

## Files affected

| Repo | File | Change |
|------|------|--------|
| `rivolution-unified-installer` | `roles/tailscale/tasks/main.yml` | New role |
| `rivolution-unified-installer` | `roles/tailscale/files/rivapi-sudoers` | New sudoers fragment |
| `rivolution-unified-installer` | `playbook.yml` (or `site.yml`) | Add role, gated by `rivolution_tailscale_enabled` |
| `rivolution` | `rivapi/config/config.go` | Add `TLSCert`, `TLSKey` vars |
| `rivolution` | `rivapi/main.go` | Conditional TLS listener |
| `rivolution` | `rivapi/dashboard/` | New "Network" section handler + templates |

## Verification

1. Installer role runs without errors on a fresh Ubuntu 26.04 install
   with `rivolution_tailscale_enabled: true`; `tailscaled` is enabled
   but stopped after the play completes.
2. Dashboard "Connect to tailnet" form: pasting a valid pre-auth key
   connects the host; status panel shows tailnet IP and hostname.
3. "Provision TLS cert" button: cert and key files appear in
   `RIVAPI_TLS_CERT_DIR`; `rivapi` restarts with TLS; browser confirms
   valid cert for the MagicDNS hostname.
4. Attempting to provision a cert on a tailnet without HTTPS Certs
   enabled shows the "Enable in admin console" message rather than an
   opaque error.
