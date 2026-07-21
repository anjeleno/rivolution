#!/bin/sh
# Launches VLC targeting Rivendell's system-scope PipeWire/JACK graph
# (rivendell_0, stereo_tool.*, ffmpeg-*) instead of the desktop session's
# own per-user PipeWire instance -- confirmed 2026-07-21 that these are
# two entirely separate graphs that never connect to each other on their
# own: a desktop-launched VLC inherits XDG_RUNTIME_DIR=/run/user/<uid>
# from its login session, while caed/Stereo Tool/rivapi all instead talk
# to /run/pipewire-system (set explicitly by their own systemd units).
# Overriding it here is safe -- both instances run as the same rd user,
# and /run/pipewire-system/pipewire-0 is owned rd:rd, so no permission
# workaround is needed, just the right environment variable.
#
# --jack-name is fixed rather than left to VLC's own default (which
# embeds its PID, e.g. vlc_12164) so the resulting JACK client/port names
# never change between launches -- /patchbay's saved link for this stays
# valid across every VLC restart instead of going stale the moment the
# PID changes (see rivapi/store/patchbay.go's own handling of this same
# problem for Stereo Tool).
export XDG_RUNTIME_DIR=/run/pipewire-system
exec /usr/bin/vlc --started-from-file --aout=jack --jack-name=vlc-rivendell "$@"
