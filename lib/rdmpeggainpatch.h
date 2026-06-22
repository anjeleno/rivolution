// rdmpeggainpatch.h
//
// Apply MP3 gain normalization by patching the encoded bitstream directly
//
//   (C) Copyright 2026 Anjeleno <la90046@gmail.com>
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

#ifndef RDMPEGGAINPATCH_H
#define RDMPEGGAINPATCH_H

#include <qobject.h>
#include <qstring.h>

//
// Applies normalization to an MP3 (MPEG Layer III) file by patching the
// global_gain field each frame already carries, via the 'mp3gain' tool,
// instead of decoding to PCM, scaling, and re-encoding. See
// docs/specs/0004-mp3-gain-patch.md for the full design and the real
// mp3gain CLI mechanics this was verified against.
//
class RDMpegGainPatch : public QObject
{
  Q_OBJECT;
 public:
  enum ErrorCode {ErrorOk=0,ErrorNotApplicable=1,ErrorToolNotFound=2,
		  ErrorToolError=3};
  RDMpegGainPatch(QObject *parent=0);
  ~RDMpegGainPatch();
  void setSourceFile(const QString &filename);
  void setDestinationFile(const QString &filename);

  // Whole dB, same contract as RDSettings::normalizationLevel() -- e.g.
  // -13 means a target peak of -13dBFS. Never hardcoded by this class.
  void setNormalizationLevel(int level);

  RDMpegGainPatch::ErrorCode patch();

  // The level actually reached (whole dB), valid after a successful
  // patch(). May differ from the requested level: gain only lands on
  // mp3gain's discrete ~1.505dB global_gain steps, and a gain increase
  // may be capped short of the request to avoid clipping.
  int achievedLevel() const;

  static QString errorText(RDMpegGainPatch::ErrorCode err);

 private:
  bool MeasurePeak(double *max_amplitude);
  QString patch_source_file;
  QString patch_destination_file;
  int patch_normalization_level;
  int patch_achieved_level;
};


#endif  // RDMPEGGAINPATCH_H
