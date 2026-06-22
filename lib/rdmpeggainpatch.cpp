// rdmpeggainpatch.cpp
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

#include <math.h>

#include <qfile.h>
#include <qprocess.h>
#include <qstringlist.h>

#include <rdmpeggainpatch.h>

//
// mp3gain's own discrete gain unit: each global_gain step changes
// amplitude by exactly 2^(1/4); see docs/specs/0004-mp3-gain-patch.md.
//
#define MP3GAIN_BINARY "/usr/bin/mp3gain"
#define MP3GAIN_STEP_DB (20.0*log10(pow(2.0,0.25)))
#define MP3GAIN_FULL_SCALE 32768.0

RDMpegGainPatch::RDMpegGainPatch(QObject *parent)
  : QObject(parent)
{
  patch_normalization_level=0;
  patch_achieved_level=0;
}


RDMpegGainPatch::~RDMpegGainPatch()
{
}


void RDMpegGainPatch::setSourceFile(const QString &filename)
{
  patch_source_file=filename;
}


void RDMpegGainPatch::setDestinationFile(const QString &filename)
{
  patch_destination_file=filename;
}


void RDMpegGainPatch::setNormalizationLevel(int level)
{
  patch_normalization_level=level;
}


int RDMpegGainPatch::achievedLevel() const
{
  return patch_achieved_level;
}


bool RDMpegGainPatch::MeasurePeak(double *max_amplitude)
{
  if(!QFile::exists(MP3GAIN_BINARY)) {
    return false;
  }

  QStringList args;
  args.push_back("-q");
  args.push_back("-s");
  args.push_back("r");    // Force a fresh analysis, ignore any stored tag
  args.push_back("-o");   // Tab-delimited, database-friendly output
  args.push_back("-x");   // Max amplitude only -- skip the loudness
                           // suggestion calculation, which targets a
                           // different (ReplayGain-style) reference than
                           // Rivendell's peak-based dBFS target and is
                           // never used by this class.
  args.push_back(patch_source_file);

  QProcess *proc=new QProcess(this);
  proc->start(MP3GAIN_BINARY,args);
  proc->waitForFinished(-1);
  if(proc->exitStatus()!=QProcess::NormalExit) {
    delete proc;
    return false;
  }
  QString output=QString(proc->readAllStandardOutput());
  delete proc;

  //
  // Two header columns in, then one data row per input file:
  // File / MP3 gain / dB gain / Max Amplitude / Max global_gain / Min global_gain
  //
  QStringList lines=output.split("\n",QString::SkipEmptyParts);
  if(lines.size()<2) {
    return false;
  }
  QStringList fields=lines.at(1).split("\t");
  if(fields.size()<4) {
    return false;
  }
  bool ok=false;
  double amp=fields.at(3).toDouble(&ok);
  if((!ok)||(amp<=0.0)) {
    return false;
  }
  *max_amplitude=amp;

  return true;
}


RDMpegGainPatch::ErrorCode RDMpegGainPatch::patch()
{
  double max_amplitude=0.0;

  patch_achieved_level=0;

  if(!QFile::exists(MP3GAIN_BINARY)) {
    return RDMpegGainPatch::ErrorToolNotFound;
  }
  if(!MeasurePeak(&max_amplitude)) {
    return RDMpegGainPatch::ErrorNotApplicable;
  }

  //
  // Same peak-dBFS formula already in production for the PCM path
  // (lib/rdaudioconvert.cpp, Stage2Convert()) -- kept numerically
  // consistent so a gain-patched file lands at the same target as one
  // normalized via the full decode/re-encode path.
  //
  double peak_sample=max_amplitude/MP3GAIN_FULL_SCALE;
  double peak_dbfs=20.0*log10(peak_sample);
  double gain_db=((double)patch_normalization_level)/100.0-peak_dbfs;
  int step_count=(int)lround(gain_db/MP3GAIN_STEP_DB);

  //
  // Clipping safety: mp3gain's own '-k' does not constrain a manually
  // specified '-g' value (verified directly against the real tool, see
  // docs/specs/0004-mp3-gain-patch.md), so a requested gain *increase*
  // has to be capped here, using the peak just measured, to whatever
  // keeps the resulting peak at or under full scale.
  //
  if(step_count>0) {
    int max_safe_steps=
      (int)floor(4.0*(log(MP3GAIN_FULL_SCALE/max_amplitude)/log(2.0)));
    if(step_count>max_safe_steps) {
      step_count=max_safe_steps;
    }
  }

  if(!QFile::exists(patch_source_file)) {
    return RDMpegGainPatch::ErrorNotApplicable;
  }
  QFile::remove(patch_destination_file);
  if(!QFile::copy(patch_source_file,patch_destination_file)) {
    return RDMpegGainPatch::ErrorToolError;
  }

  QStringList args;
  args.push_back("-q");
  args.push_back("-c");   // Never block on an interactive clip-risk
                           // confirmation prompt -- fatal with no
                           // controlling tty.
  args.push_back("-g");
  args.push_back(QString::asprintf("%d",step_count));
  args.push_back(patch_destination_file);

  QProcess *proc=new QProcess(this);
  proc->start(MP3GAIN_BINARY,args);
  proc->waitForFinished(-1);
  bool ok=(proc->exitStatus()==QProcess::NormalExit)&&(proc->exitCode()==0);
  delete proc;
  if(!ok) {
    QFile::remove(patch_destination_file);
    return RDMpegGainPatch::ErrorToolError;
  }

  patch_achieved_level=(int)lround((peak_dbfs+step_count*MP3GAIN_STEP_DB)*100.0);

  return RDMpegGainPatch::ErrorOk;
}


QString RDMpegGainPatch::errorText(RDMpegGainPatch::ErrorCode err)
{
  QString ret=QObject::tr("Unknown Error");

  switch(err) {
  case RDMpegGainPatch::ErrorOk:
    ret=QObject::tr("OK");
    break;

  case RDMpegGainPatch::ErrorNotApplicable:
    ret=QObject::tr("Gain patch not applicable to this file");
    break;

  case RDMpegGainPatch::ErrorToolNotFound:
    ret=QObject::tr("mp3gain is not installed");
    break;

  case RDMpegGainPatch::ErrorToolError:
    ret=QObject::tr("mp3gain reported an error");
    break;
  }

  return ret;
}
