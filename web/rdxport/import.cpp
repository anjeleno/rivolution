// import.cpp
//
// Rivendell web service portal -- Import service
//
//   (C) Copyright 2010-2021 Fred Gleason <fredg@paravelsystems.com>
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

#include <stdio.h>
#include <stdlib.h>
#include <sys/types.h>
#include <sys/stat.h>
#include <fcntl.h>

#include <QFile>

#include <rdapplication.h>
#include <rdaudioconvert.h>
#include <rdcart.h>
#include <rdconf.h>
#include <rdformpost.h>
#include <rdgroup.h>
#include <rdhash.h>
#include <rdlibrary_conf.h>
#include <rdmpeggainpatch.h>
#include <rdsettings.h>
#include <rdweb.h>

#include "rdxport.h"

void Xport::Import()
{
  unsigned length_deviation=0;
  unsigned msecs=0;
  int resp_code=0;
  QString remote_host;
  QString err_msg;

  if(getenv("REMOTE_HOST")==NULL) {
    if(getenv("REMOTE_ADDR")==NULL) {
      XmlExit("Internal server error",500,"import.cpp",LINE_NUMBER);
    }
    else {
      remote_host=getenv("REMOTE_ADDR");
    }
  }
  else {
    remote_host=getenv("REMOTE_HOST");
  }

  //
  // Verify Post
  //
  int cartnum=0;
  if(!xport_post->getValue("CART_NUMBER",&cartnum)) {
    XmlExit("Missing CART_NUMBER",400,"import.cpp",LINE_NUMBER);
  }
  int cutnum=0;
  if(!xport_post->getValue("CUT_NUMBER",&cutnum)) {
    XmlExit("Missing CUT_NUMBER",400,"import.cpp",LINE_NUMBER);
  }
  int channels=0;
  if(!xport_post->getValue("CHANNELS",&channels)) {
    XmlExit("Missing CHANNELS",400,"import.cpp",LINE_NUMBER);
  }
  int normalization_level=0;
  if(!xport_post->getValue("NORMALIZATION_LEVEL",&normalization_level)) {
    XmlExit("Missing NORMALIZATION_LEVEL",400,"import.cpp",LINE_NUMBER);
  }
  int autotrim_level=0;
  if(!xport_post->getValue("AUTOTRIM_LEVEL",&autotrim_level)) {
    XmlExit("Missing AUTOTRIM_LEVEL",400,"import.cpp",LINE_NUMBER);
  }
  int use_metadata=0;
  if(!xport_post->getValue("USE_METADATA",&use_metadata)) {
    XmlExit("Missing USE_METADATA",400,"import.cpp",LINE_NUMBER);
  }
  int create=0;
  if(!xport_post->getValue("CREATE",&create)) {
    create=0;
  }
  int format_override=-1;
  if(!xport_post->getValue("FORMAT",&format_override)) {
    format_override=-1;
  }
  QString group_name;
  xport_post->getValue("GROUP_NAME",&group_name);
  QString title;
  xport_post->getValue("TITLE",&title);
  QString filename;
  if(!xport_post->getValue("FILENAME",&filename)) {
    XmlExit("Missing FILENAME",400,"import.cpp",LINE_NUMBER);
  }
  if(!xport_post->isFile("FILENAME")) {
    XmlExit("Missing file data",400,"import.cpp",LINE_NUMBER);
  }

  //
  // Verify User Perms
  //
  if(create&&(cartnum==0)&&(cutnum==0)) {
    if(!rda->user()->groupAuthorized(group_name)) {
      XmlExit("No such group",404,"import.cpp",LINE_NUMBER);
    }
  }
  else {
    if(cartnum==0) {
      XmlExit("No such cart",404,"import.cpp",LINE_NUMBER);
    }
    if(cutnum==0) {
      XmlExit("No such cut",404,"import.cpp",LINE_NUMBER);
    }
    if(RDCart::exists(cartnum)) {
      if(!rda->user()->cartAuthorized(cartnum)) {
	XmlExit("No such cart",404,"import.cpp",LINE_NUMBER);
      }
    }
    else {
      XmlExit("No such cart",404,"import.cpp",LINE_NUMBER);
    }
  }
  if(!rda->user()->editAudio()) {
    XmlExit("Forbidden",404,"import.cpp",LINE_NUMBER);
  }
  if(create&&(!rda->user()->createCarts())) {
    XmlExit("Forbidden",404,"import.cpp",LINE_NUMBER);
  }

  //
  // Verify Title Uniqueness
  //
  if(!title.isEmpty()) {
    if((!rda->system()->allowDuplicateCartTitles())&&
       (!rda->system()->fixDuplicateCartTitles())&&
       (!RDCart::titleIsUnique(cartnum,title))) {
      XmlExit("Duplicate Cart Title Not Allowed",404,"import.cpp",LINE_NUMBER);
    }
  }

  //
  // Load Configuration
  //
  RDCart *cart=NULL;
  RDCut *cut=NULL;
  if(cartnum==0) {
    RDGroup *group=new RDGroup(group_name);
    if(!group->exists()) {
      XmlExit("No such group",404,"import.cpp",LINE_NUMBER);
    }
    if((cartnum=group->nextFreeCart())==0) {
      XmlExit("No available carts for specified group",404,"import.cpp",LINE_NUMBER);
    }
    cart=new RDCart(cartnum);
    if(RDCart::create(group_name,RDCart::Audio,&err_msg,cartnum)==0) {
      delete cart;
      XmlExit("Unable to create cart ["+err_msg+"]",500,"import.cpp",
	      LINE_NUMBER);
    }
    SendNotification(RDNotification::CartType,RDNotification::AddAction,
		     QVariant(cartnum));
    cutnum=1;
    cut=new RDCut(cartnum,cutnum,true);
    delete group;
  }
  else {
    cart=new RDCart(cartnum);
    cut=new RDCut(cartnum,cutnum);
  }
  if(!RDCart::exists(cartnum)) {
    XmlExit("No such cart",404,"import.cpp",LINE_NUMBER);
  }
  if(!RDCut::exists(cartnum,cutnum)) {
    XmlExit("No such cut",404,"import.cpp",LINE_NUMBER);
  }
  RDLibraryConf *conf=new RDLibraryConf(rda->config()->stationName());
  RDSettings *settings=new RDSettings();
  unsigned effective_format=conf->defaultFormat();
  if((format_override>=0)&&(format_override<=3)) {
    effective_format=format_override;
  }
  switch(effective_format) {
  case 0:
    settings->setFormat(RDSettings::Pcm16);
    break;

  case 1:
    settings->setFormat(RDSettings::MpegL2Wav);
    break;

  case 2:
    settings->setFormat(RDSettings::Pcm24);
    break;

  case 3:
    settings->setFormat(RDSettings::MpegL3);
    break;
  }
  settings->setChannels(channels);
  settings->setSampleRate(rda->system()->sampleRate());
  settings->setBitRate(channels*conf->defaultBitrate());
  settings->setNormalizationLevel(normalization_level);
  RDWaveData wavedata;
  RDWaveFile *wave=new RDWaveFile(filename);
  if(!wave->openWave(&wavedata)) {
    delete wave;
    XmlExit("Format Not Supported",415,"import.cpp",LINE_NUMBER);
  }
  bool source_is_mp3=(wave->getHeadLayer()==3);
  unsigned source_sample_rate=wave->getSamplesPerSec();
  delete wave;
  if(use_metadata) {
    if((!rda->system()->allowDuplicateCartTitles())&&
       (!rda->system()->fixDuplicateCartTitles())&&
       (!RDCart::titleIsUnique(cartnum,wavedata.title()))) {
      XmlExit("Duplicate Cart Title Not Allowed",404,"import.cpp",LINE_NUMBER);
    }
  }

  //
  // True passthrough: whenever the source is genuinely MP3 (MPEG audio
  // layer 3), the effective target format is also MP3, and the source's
  // real sample rate already matches the system rate, there is never a
  // reason to decode and re-encode an MP3 back to MP3. The sample-rate
  // check matters because caed's MPEG playback path
  // (cae/driver_alsa.cpp) does not resample mismatched-rate MPEG audio
  // at playout time, unlike its PCM/Vorbis paths -- a passthrough copy
  // of a file recorded at a different rate than the system's would play
  // back pitch-shifted. When either of these don't hold, fall through
  // to the normal conversion path below, which resamples correctly.
  //
  // A requested normalization doesn't rule passthrough out by itself --
  // see the gain-patch attempt below (docs/specs/0004-mp3-gain-patch.md):
  // normalization can be applied directly to the MP3 bitstream, without
  // decoding/re-encoding, whenever that succeeds. Neither does a
  // requested autotrim: RDWaveFile::startTrim()/endTrim() (via
  // GetEnergy()/LoadEnergyMpegLayer3()) already decode MP3 in memory to
  // measure real sample-accurate trim points, no re-encode required --
  // the same libmad-based decode already run against every passthrough
  // import's destination file for LEVL/peak persistence (the
  // wave->hasEnergy() call below). That call's PutLevl() persists the
  // resulting energy data into the file's own LEVL chunk, so the
  // separate RDWaveFile that autoTrim() opens afterward finds it
  // already there (via GetLevl()) and never has to decode a second
  // time.
  //
  bool passthrough_eligible=source_is_mp3&&(effective_format==3)&&
    (source_sample_rate==rda->system()->sampleRate());
  bool do_passthrough=passthrough_eligible&&(normalization_level==0);
  bool do_gain_patch=passthrough_eligible&&(normalization_level!=0);
  QString passthrough_source_file=filename;
  RDAudioConvert::ErrorCode conv_err=RDAudioConvert::ErrorOk;
  RDAudioConvert *conv=NULL;

  if(do_gain_patch) {
    RDMpegGainPatch *gainpatch=new RDMpegGainPatch();
    QString patched_file=filename+".gainpatch";
    gainpatch->setSourceFile(filename);
    gainpatch->setDestinationFile(patched_file);
    gainpatch->setNormalizationLevel(normalization_level);
    RDMpegGainPatch::ErrorCode gainpatch_err=gainpatch->patch();
    if(gainpatch_err==RDMpegGainPatch::ErrorOk) {
      passthrough_source_file=patched_file;
      do_passthrough=true;  // Reuses the WAV-wrap-and-finish block below.
      if(abs(gainpatch->achievedLevel()-normalization_level)>1) {
	// More than a single ~1.5dB global_gain step off the requested
	// level -- a real clipping-safety cap, not just the ordinary
	// (at most half-a-step) discrete-step rounding every gain-patch
	// import has. Worth a log line; routine rounding isn't.
	rda->syslog(LOG_INFO,
		   "rdxport: MP3 gain-patch normalization capped for cart "
		   "%d, cut %d -- requested %ddBFS, achieved %ddBFS",
		   cartnum,cutnum,normalization_level,
		   gainpatch->achievedLevel());
      }
    }
    else {
      rda->syslog(LOG_INFO,
		 "rdxport: MP3 gain-patch normalization not applied for "
		 "cart %d, cut %d (%s) -- using full conversion instead",
		 cartnum,cutnum,
		 RDMpegGainPatch::errorText(gainpatch_err).toUtf8().
		 constData());
      //
      // The full conversion below decodes the source, so re-encoding
      // it back into another lossy format (MP3, the passthrough target
      // implied by effective_format/format_override) would add a
      // second generation of lossy compression on top of whatever the
      // gain-patch was trying to avoid in the first place. Fall back to
      // the station's own real configured default instead -- discarding
      // the per-request format override for this one case -- which also
      // sidesteps RDLIBRARY's DEFAULT_BITRATE being meaningless (left
      // at 0) whenever that default format isn't itself MP3/Layer 2,
      // since the RDLibrary editor only exposes a bitrate field then.
      //
      switch(conf->defaultFormat()) {
      case 0:
	settings->setFormat(RDSettings::Pcm16);
	break;

      case 1:
	settings->setFormat(RDSettings::MpegL2Wav);
	break;

      case 2:
	settings->setFormat(RDSettings::Pcm24);
	break;

      case 3:
	settings->setFormat(RDSettings::MpegL3);
	break;
      }
      settings->setBitRate(channels*conf->defaultBitrate());
    }
    delete gainpatch;
  }

  if(do_passthrough) {
    //
    // Copy the source's MPEG frame data verbatim into a WAV-wrapped
    // destination -- the audio bitstream itself is never decoded or
    // re-encoded, only the container changes, so this stays a true
    // passthrough. Wrapping it lets LEVL energy data (computed below)
    // persist in the file's own header, the same mechanism PCM/Layer II
    // already use -- a bare elementary stream has no header to put it in.
    // passthrough_source_file is either the original upload (true
    // byte-copy passthrough) or a gain-patched scratch copy (normalized
    // passthrough) -- identical handling either way from here on.
    //
    RDWaveFile *src_wave=new RDWaveFile(passthrough_source_file);
    if(!src_wave->openWave()) {
      delete src_wave;
      XmlExit("Unable to access imported file",500,"import.cpp",LINE_NUMBER,
	      RDAudioConvert::ErrorNoDestination);
    }
    RDWaveFile *dst_wave=new RDWaveFile(RDCut::pathName(cartnum,cutnum));
    dst_wave->setFormatTag(WAVE_FORMAT_MPEG);
    dst_wave->setChannels(src_wave->getChannels());
    dst_wave->setSamplesPerSec(src_wave->getSamplesPerSec());
    dst_wave->setHeadLayer(3);
    dst_wave->setHeadBitRate(src_wave->getHeadBitRate());
    dst_wave->setHeadMode(src_wave->getHeadMode());
    if(!dst_wave->createWave(&wavedata)) {
      delete src_wave;
      delete dst_wave;
      XmlExit("Unable to write imported file",500,"import.cpp",LINE_NUMBER,
	      RDAudioConvert::ErrorNoDestination);
    }
    char passthrough_buffer[65536];
    int passthrough_n;
    while((passthrough_n=
	   src_wave->readWave(passthrough_buffer,sizeof(passthrough_buffer)))>
	  0) {
      dst_wave->writeWave(passthrough_buffer,passthrough_n);
    }
    delete src_wave;
    if(passthrough_source_file!=filename) {
      QFile::remove(passthrough_source_file);  // The gain-patch scratch
                                                // copy -- not the
                                                // original upload.
    }
    dst_wave->closeWave();
    delete dst_wave;
    wave=new RDWaveFile(RDCut::pathName(cartnum,cutnum));
    if(wave->openWave()) {
      msecs=wave->getExtTimeLength();
      // dst_wave above never had real sample/frame counts (those are
      // only known once a file is closed and reopened), so the
      // decode-and-measure pass has to happen here instead, against
      // this freshly-reopened handle, for hasEnergy() (via PutLevl())
      // to persist real peak data rather than an empty LEVL chunk.
      wave->hasEnergy();
    }
    else {
      delete wave;
      XmlExit("Unable to access imported file",500,"import.cpp",LINE_NUMBER,
	      RDAudioConvert::ErrorNoDestination);
    }
    delete wave;
    cut->checkInRecording(rda->config()->stationName(),rda->user()->name(),
			  remote_host,settings,msecs);
    if(use_metadata>0) {
      cart->setMetadata(&wavedata);
    }
    cut->setMetadata(&wavedata,use_metadata);
    if(autotrim_level!=0) {
      cut->autoTrim(RDCut::AudioBoth,100*autotrim_level);
    }
    cart->updateLength();
    cart->resetRotation();
    cart->calculateAverageLength(&length_deviation);
    cart->setLengthDeviation(length_deviation);
    resp_code=200;
  }
  else {
    conv=new RDAudioConvert();
    conv->setSourceFile(filename);
    conv->setDestinationFile(RDCut::pathName(cartnum,cutnum));
    conv->setDestinationSettings(settings);
    conv_err=conv->convert();
    switch(conv_err) {
    case RDAudioConvert::ErrorOk:
      wave=new RDWaveFile(RDCut::pathName(cartnum,cutnum));
      if(wave->openWave()) {
	msecs=wave->getExtTimeLength();
      }
      else {
	delete wave;
	XmlExit("Unable to access imported file",500,"import.cpp",LINE_NUMBER);
      }
      delete wave;
      cut->checkInRecording(rda->config()->stationName(),rda->user()->name(),
			    remote_host,settings,msecs);
      if(use_metadata>0) {
	cart->setMetadata(conv->sourceWaveData());
      }
      cut->setMetadata(conv->sourceWaveData(),use_metadata);
      if(autotrim_level!=0) {
	cut->autoTrim(RDCut::AudioBoth,100*autotrim_level);
      }
      cart->updateLength();
      cart->resetRotation();
      cart->calculateAverageLength(&length_deviation);
      cart->setLengthDeviation(length_deviation);
      resp_code=200;
      break;

    case RDAudioConvert::ErrorFormatNotSupported:
    case RDAudioConvert::ErrorInvalidSettings:
      resp_code=415;
      break;

    case RDAudioConvert::ErrorNoSource:
    case RDAudioConvert::ErrorNoDestination:
    case RDAudioConvert::ErrorInvalidSource:
    case RDAudioConvert::ErrorInternal:
    case RDAudioConvert::ErrorNoSpace:
    case RDAudioConvert::ErrorNoDisc:
    case RDAudioConvert::ErrorNoTrack:
    case RDAudioConvert::ErrorInvalidSpeed:
      resp_code=500;
      break;

    case RDAudioConvert::ErrorFormatError:
      resp_code=400;
      break;
    }
  }
  if(resp_code==200) {
    cut->setSha1Hash(RDSha1HashFile(RDCut::pathName(cut->cutName())));
    if(!title.isEmpty()) {
      cart->setTitle(title);
    }
    printf("Content-type: application/xml; charset=utf-8\n");
    printf("Status: %d\n",resp_code);
    printf("\n");
    printf("<RDWebResult>\r\n");
    printf("  <ResponseCode>%d</ResponseCode>\r\n",resp_code);
    printf("  <ErrorString>OK</ErrorString>\r\n");
    printf("  <CartNumber>%d</CartNumber>\r\n",cartnum);
    printf("  <CutNumber>%d</CutNumber>\r\n",cutnum);
    printf("</RDWebResult>\r\n");
    SendNotification(RDNotification::CartType,RDNotification::ModifyAction,
		     QVariant(cartnum));
    unlink(filename.toUtf8());
    rmdir(xport_post->tempDir().toUtf8());
    exit(0);
  }
  XmlExit(RDAudioConvert::errorText(conv_err),resp_code,"import.cpp",
	  LINE_NUMBER,conv_err);
}
