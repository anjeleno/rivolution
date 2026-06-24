#!/bin/bash
# Creates the missing 'rivendell' system user/group (and 'pypad', for
# completeness), fixes /var/snd ownership, recreates the Rivendell MySQL
# database empty (RDDBConfig's GUI path seeds it via a bare 'rddbmgr
# --create', which leaves no room for '--generate-audio' to run
# afterward since rddbmgr refuses to touch a non-empty database), runs
# the schema/seed/test-tone generation in one shot from the CLI,
# disables PulseAudio in favor of direct ALSA access, and enables
# rivendell.service at boot. All for a from-source Rivendell build that
# never ran the .deb package's postinst script.
#
# UID/GID 150/151 are upstream's own fixed values (debian/postinst),
# not arbitrary -- kept for permission consistency across any future
# cloned/networked Rivendell hosts.
#
# Run with: sudo bash fix-rivendell-user.sh

set -e

getent group rivendell >/dev/null || groupadd -r -g 150 rivendell
id rivendell >/dev/null 2>&1 || useradd -o -u 150 -g rivendell -s /bin/false -r -c "Rivendell radio automation system" -d /var/snd rivendell
getent group pypad >/dev/null || groupadd -r -g 151 pypad
id pypad >/dev/null 2>&1 || useradd -o -u 151 -g pypad -s /bin/false -r -c "Rivendell PyPAD scripts" -d /dev/null pypad
mkdir -p /var/snd
chown rivendell:rivendell /var/snd
chmod 775 /var/snd

# Copy rd.conf sample file -- resolved relative to this script's own
# location (repo_root/scripts/this_file -> repo_root/conf/rd.conf-sample)
# rather than a hardcoded path, since the repo can be cloned anywhere.
script_dir=$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)
cp "$script_dir/../conf/rd.conf-sample" /etc/rd.conf

# Generate a long, unique password for 'rduser' -- the sample ships with
# a fixed, public default ('hackme') -- and write it into rd.conf's
# [mySQL] section so it's the one actually used below and by Rivendell
# itself afterward.
mysql_pass=$(tr -dc 'A-Za-z0-9' < /dev/urandom | head -c 32)
sed -i "/^\[mySQL\]/,/^\[/{s/^Password=.*/Password=$mysql_pass/}" /etc/rd.conf

# Read the remaining MySQL connection parameters straight from rd.conf's
# [mySQL] section -- the same values RDConfig itself uses -- rather than
# hardcoding them here.
mysql_host=$(awk -F= '/^\[mySQL\]/{f=1;next}/^\[/{f=0}f&&$1=="Hostname"{print $2}' /etc/rd.conf)
mysql_user=$(awk -F= '/^\[mySQL\]/{f=1;next}/^\[/{f=0}f&&$1=="Loginname"{print $2}' /etc/rd.conf)
mysql_db=$(awk -F= '/^\[mySQL\]/{f=1;next}/^\[/{f=0}f&&$1=="Database"{print $2}' /etc/rd.conf)

# Recreate the database empty and re-grant 'rduser' -- mirrors
# RDDBConfig's CreateDb::create() (utils/rddbconfig/createdb.cpp) minus
# its trailing 'rddbmgr --create' call, so the schema/seed/test-tone
# step below can run against a genuinely empty database.
mysql -h "$mysql_host" <<SQL
drop database if exists \`$mysql_db\`;
create database if not exists \`$mysql_db\`;
drop user if exists '$mysql_user'@'%';
drop user if exists '$mysql_user'@'localhost';
create user '$mysql_user'@'%' identified by '$mysql_pass';
create user '$mysql_user'@'localhost' identified by '$mysql_pass';
grant select,insert,update,delete,create,drop,index,alter,lock tables on \`$mysql_db\`.* to '$mysql_user'@'%';
grant select,insert,update,delete,create,drop,index,alter,lock tables on \`$mysql_db\`.* to '$mysql_user'@'localhost';
flush privileges;
SQL

# Creates the schema, seeds default data, generates the test-tone audio
# file, and adds it to the library/audio store -- all in one pass since
# the database above is empty.
rddbmgr --create --generate-audio

# Disable PulseAudio and configure audio priorities, so caed/ALSA get
# uncontended, real-time access to the sound device instead of fighting
# PulseAudio for it.
killall pulseaudio || true
sed -i 's/# autospawn = yes/autospawn = no/' /etc/pulse/client.conf
gpasswd -d pulse audio || true
usermod -aG audio rd
usermod -aG audio rivendell
grep -qF '@audio      hard      memlock     unlimited' /etc/security/limits.conf || cat >> /etc/security/limits.conf <<'EOF'
@audio      hard      rtprio          90
@audio      hard      memlock     unlimited
EOF

systemctl enable rivendell.service
systemctl restart rivendell.service

echo "Done. Verify with: getent passwd rivendell; getent group rivendell; ls -ld /var/snd; systemctl is-enabled rivendell"
