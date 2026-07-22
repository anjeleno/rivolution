```
██████╗ ██╗ ██╗   ██╗  ██████╗  ██╗      ██╗   ██╗ ████████╗ ██╗  ██████╗  ███╗   ██╗
██╔══██╗██║ ██║   ██║ ██╔═══██╗ ██║      ██║   ██║ ╚══██╔══╝ ██║ ██╔═══██╗ ████╗  ██║
██████╔╝██║ ██║   ██║ ██║   ██║ ██║      ██║   ██║    ██║    ██║ ██║   ██║ ██╔██╗ ██║
██╔══██╗██║ ╚██╗ ██╔╝ ██║   ██║ ██║      ██║   ██║    ██║    ██║ ██║   ██║ ██║╚██╗██║
██║  ██║██║  ╚████╔╝  ╚██████╔╝ ███████╗ ╚██████╔╝    ██║    ██║ ╚██████╔╝ ██║ ╚████║
╚═╝  ╚═╝╚═╝   ╚═══╝    ╚═════╝  ╚══════╝  ╚═════╝     ╚═╝    ╚═╝  ╚═════╝  ╚═╝  ╚═══╝
```

![Latest release](https://img.shields.io/github/v/release/anjeleno/rivolution?label=release&color=blue)
![Platform](https://img.shields.io/badge/platform-Ubuntu%2024.04%20%7C%2026.04-orange)

**[rivolution.dev](https://rivolution.dev/)** — the project website.

## Pages

| Page | What it's for |
| --- | --- |
| [[Start Here\|Start-Here]] | OS/desktop prerequisites — updates, hostname, timezone, the `rd` user, MATE, xRDP (including the Wayland fix for a fresh 26.04 install). Start here regardless of install method. |
| [[Deb Package Install\|Deb-Package-Install]] | Install the released `.deb` and get a station running: audio driver, Program Source, VLC routing. |
| [[Build From Source\|Build-From-Source]] | Build your own `.deb` from a checkout (`rebuild-deb.sh`) instead of downloading one — the only from-source path that gets full `postinst` automation. |
| [[Unified Installer\|Unified-Installer]] | The Ansible playbook that automates everything above end to end, standalone/server/client. |
| [[Web Dashboard\|Web-Dashboard]] | Walkthrough of `rivapi`, the browser-based dashboard — service control, streaming, patchbay, mode, tasks, backup. |
| [[Segue Back-Timing\|Segue-Back-Timing]] | How segue back-timing keeps a produced element's tail from colliding with the next song's intro. |

## Reference

These live in the repo itself, not duplicated here, so a fix's commit
can update them in the same diff:

- [Known Issues](https://github.com/anjeleno/rivolution/blob/main/KNOWN_ISSUES.md)
- [Backlog](https://github.com/anjeleno/rivolution/blob/main/BACKLOG.md)
- [Roadmap](https://github.com/anjeleno/rivolution/blob/main/ROADMAP.md)
- [Changelog](https://github.com/anjeleno/rivolution/blob/main/CHANGELOG.md)
