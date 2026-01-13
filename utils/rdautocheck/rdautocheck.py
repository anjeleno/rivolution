#!%PYTHON_BANGPATH%

# rdautocheck.py
#
# Read metadata for Rivendell backup sets.
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
import subprocess
import sys
import syslog

def UnmountDevice(mntpt):
    if(os.system(command='findmnt '+mntpt+' > /dev/null')==0):
        os.system(command='umount '+mntpt+' > /dev/null')
    try:
        os.rmdir(mntpt)
    except FileNotFoundError:
        pass

def PrintMetadata(mntpt):
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
        UnmountDevice(mntpt)
        return
    os.system(command='sleep 5')

    #
    # Read Info File
    #
    try:
        with open(mntpt+'/INFO.txt','r') as f:
            print(mntpt+':')
            print(f.read())
            f.close()
    except FileNotFoundError:
        print(mntpt+':')
        print('[no metadata found]')
        print()

    #
    # Unmount backup device
    #
    UnmountDevice(mntpt)

USAGE='rdautocheck.py <backup-mountpoint1> [<backup-mountpoint2>] [...]'

if(len(sys.argv)<2):
    print(USAGE)
    exit(1)
for arg in range(1,len(sys.argv)):
    PrintMetadata(sys.argv[arg])
