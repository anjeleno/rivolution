#!%PYTHON_BANGPATH%

# rdautoback.py
#
# Unattended backup script for Rivendell
#
#   (C) Copyright 2024 Fred Gleason <fredg@paravelsystems.com>
#
#   This program is free software; you can redistribute it and/or modify
#   it under the terms of the GNU General Public License version 2 as
#   published by the Free Software Foundation.
#
#   This program is distributed in the hope that it will be useful,
#   but WITHOUT ANY WARRANTY; without even the implied warranty of
#   MERCHANTABILITY or FITNESS FOR A PARTICULAR PURPOSE.  See the
#   GNU General Public License for more details.
#
#   You should have received a copy of the GNU General Public
#   License along with this program; if not, write to the Free Software
#   Foundation, Inc., 675 Mass Ave, Cambridge, MA 02139, USA.
#

import configparser
import datetime
import MySQLdb
import os
from pathlib import Path
import sys
import syslog

def UnmountDevice(mntpt):
    if(os.system(command='findmnt '+mntpt+' > /dev/null')==0):
        os.system(command='umount '+mntpt+' > /dev/null')
    try:
        os.rmdir(mntpt)
    except FileNotFoundError:
        pass

def BackupMountpoint(mntpt):
    #
    # Proactively unmount to make sure we get a clean volume
    #
    UnmountDevice(mntpt)

    #
    # Mount backup device
    #
    Path(mntpt).mkdir(parents=True,exist_ok=True)
    result=os.WEXITSTATUS(os.system(command='mount '+mntpt+" 2> /dev/null"))
    if((result!=0)and(result!=64)):
        print(mntpt+': [not found]')
        print()
        return
    os.system(command='sleep 5')
    syslog.syslog(syslog.LOG_INFO,'Starting Rivendell backup to "'+mntpt+'"')

    #
    # Dump database
    #
    cmd='mysqldump -h '+rd_config.get('mySQL','Hostname')+' -u '+rd_config.get('mySQL','Loginname')+' -p'+rd_config.get('mySQL','Password')+' '+rd_config.get('mySQL','Database')+' | gzip > '+mntpt+'/db.sql.gz'
    os.system(command=cmd)

    #
    # Copy Audio Store
    #
    cmd='rsync -av --delete /var/snd '+mntpt
    os.system(command=cmd)

    #
    # Generate Info File
    #
    with open(mntpt+'/INFO.txt','w') as f:
        f.write('[Backup]\n')
        f.write('Name='+mntpt+'\n')
        with os.popen('date --iso-8601=seconds',mode='r') as f1:
            f.write('DateTime='+f1.read())
            f1.close()
        try:
            db=MySQLdb.connect(user=rd_config.get('mySQL','Loginname'),
                               passwd=rd_config.get('mySQL','Password'),
                               host=rd_config.get('mySQL','Hostname'),
                               database=rd_config.get('mySQL','Database'),
                               charset='utf8mb4')
        except TypeError:
            db=MySQLdb.connect(user=rd_config.get('mySQL','Loginname'),
                               password=rd_config.get('mySQL','Password'),
                               host=rd_config.get('mySQL','Hostname'),
                               database=rd_config.get('mySQL','Database'),
                               charset='utf8mb4')
        cursor=db.cursor()
        cursor.execute('select `DB` from `VERSION`')
        db_ver=cursor.fetchone()[0]
        f.write('DatabaseSchema='+str(db_ver)+'\n')
        if(db_ver>=353):
            cursor.execute('select `REALM_NAME` from `SYSTEM`')
            f.write('RealmName='+cursor.fetchone()[0]+'\n')
        else:
            f.write('RealmName=NULL\n')
        db.close()
        with os.popen('du -hd 0 '+mntpt+'/snd',mode='r') as f1:
            values=f1.read().split('\t')
            f.write('AudioStorage='+values[0]+'\n')
            f1.close()
        with os.popen('ls -1 '+mntpt+'/snd | wc -l') as f1:
            f.write('AudioFiles='+f1.read())
            f1.close()
        f.close()

    #
    # Unmount backup device
    #
    UnmountDevice(mntpt)
    #os.system(command='umount '+mntpt)
    #os.rmdir(mntpt)

    syslog.syslog(syslog.LOG_INFO,'Completed Rivendell backup to "'+mntpt+'"')

USAGE='rdbackup.py <backup-mntpt1> [<backup-mntpt2>] [...]'

if(len(sys.argv)<2):
    print(USAGE)
    exit(1)

#
# Load rd.conf(5)
#
rd_config=configparser.ConfigParser(interpolation=None)
rd_config.read_file(open('/etc/rd.conf'))

#
# Open the syslog
#
syslog.openlog('rdautoback.py',logoption=syslog.LOG_PERROR,facility=int(rd_config.get('Identity','SyslogFacility',fallback=syslog.LOG_USER)))

for arg in range(1,len(sys.argv)):
    BackupMountpoint(sys.argv[arg])
