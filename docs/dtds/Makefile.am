## Makefile.am
##
## docs/dtds/Makefile.am for Rivendell
##
## (C) Copyright 2020-2023 Fred Gleason <fredg@paravelsystems.com>
##
##   This program is free software; you can redistribute it and/or modify
##   it under the terms of the GNU General Public License version 2 as
##   published by the Free Software Foundation.
##
##   This program is distributed in the hope that it will be useful,
##   but WITHOUT ANY WARRANTY; without even the implied warranty of
##   MERCHANTABILITY or FITNESS FOR A PARTICULAR PURPOSE.  See the
##   GNU General Public License for more details.
##
##   You should have received a copy of the GNU General Public
##   License along with this program; if not, write to the Free Software
##   Foundation, Inc., 675 Mass Ave, Cambridge, MA 02139, USA.
##
## Use automake to process this into a Makefile.in

##
## Build Dependencies
##
%.html:	%.xml
	xsltproc -o $@ $(top_srcdir)/helpers/docbook/xhtml/docbook.xsl $<
%.pdf:	%.xml
	xsltproc $(top_srcdir)/helpers/docbook/fo/docbook.xsl $< | JAVA_TOOL_OPTIONS="-XX:-TieredCompilation" fop - -pdf $@

all-local:	direct.html\
                direct.pdf\
                rivendell.html\
                rivendell.pdf\
                superfeed.html\
                superfeed.pdf

EXTRA_DIST = direct.html\
             direct.pdf\
             direct.xml\
             rivendell.html\
             rivendell.pdf\
             rivendell.xml\
             superfeed.html\
             superfeed.pdf\
             superfeed.xml

CLEANFILES = *~
MAINTAINERCLEANFILES = *~\
                       *.1\
                       *.8\
                       *.html\
                       *.pdf\
                       aclocal.m4\
                       configure\
                       Makefile.in
