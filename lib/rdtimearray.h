// rdtimearray.h
//
// Record a sequence of precise points in time with microsecond precision.
//
//   (C) Copyright 2025 Fred Gleason <fredg@paravelsystems.com>
//
//   This program is free software; you can redistribute it and/or modify
//   it under the terms of the GNU General Public License version 2 as
//   published by the Free Software Foundation.
//
//   This program is distributed in the hope that it will be useful,
//   but WITHOUT ANY WARRANTY; without even the implied warranty of
//   MERCHANTABILITY or FITNESS FOR A PARTICULAR PURPOSE.  See the
//   GNU General Public License for more details.
//
//   You should have received a copy of the GNU General Public
//   License along with this program; if not, write to the Free Software
//   Foundation, Inc., 675 Mass Ave, Cambridge, MA 02139, USA.
//

#ifndef RDTIMEARRAY_H
#define RDTIMEARRAY_H

#include <stdint.h>

#include <QList>
#include <QString>

class RDTimePoint
{
 public:
  RDTimePoint(const QString &label);
  QString label() const;
  int64_t usecs() const;
  QString toString() const;
  int64_t operator-(const RDTimePoint &rhs) const;

 private:
  QString d_label;
  int64_t d_usecs;
};


class RDTimeArray
{
 public:
  RDTimeArray();
  int size() const;
  RDTimePoint timePoint(int n) const;
  void addPoint(QString label="");
  QString toString(int from=0,int to=-1) const;
  QString offsetsToString(int from=1,int to=-1) const;
  int64_t usecsElapsed(int from=0,int to=-1) const;
  
 private:
  QList<RDTimePoint> d_points;
};


#endif  // RDTIMEARRAY_H
