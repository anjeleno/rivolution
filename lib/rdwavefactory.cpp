// rdwavefactory.cpp
//
// Factory for generating audio waveform pixmaps
//
//   (C) Copyright 2021 Fred Gleason <fredg@paravelsystems.com>
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

#include <QObject>
#include <QPainter>

#include "rdapplication.h"
#include "rdconf.h"
#include "rdcut.h"
#include "rdpeaksexport.h"
#include "rdwavefactory.h"

RDWaveFactory::RDWaveFactory(RDWaveFactory::TrackMode mode)
{
  d_track_mode=mode;
  d_cart_number=0;
  d_cut_number=-1;

  d_font_engine=new RDFontEngine();
}


RDWaveFactory::~RDWaveFactory()
{
  delete d_font_engine;
}


RDWaveFactory::TrackMode RDWaveFactory::trackMode() const
{
  return d_track_mode;
}


unsigned RDWaveFactory::cartNumber() const
{
  return d_cart_number;
}


int RDWaveFactory::cutNumber() const
{
  return d_cut_number;
}


QPixmap RDWaveFactory::generate(int height,int x_shrink,int gain,
				bool incl_scale,int start_col,int width)
{
  //
  // start_col/width let the caller render just a horizontal slice of the
  // full waveform (in the same pixel-column space the whole-file pixmap
  // would use) instead of the whole cut in one QPixmap. This exists so a
  // caller can tile a long cut into several pixmaps that each stay safely
  // under Qt/X11's ~32767px maximum single-bitmap width -- rendering the
  // full width in one QPixmap silently truncates or corrupts once that
  // limit is exceeded (Rivendell issue #835 upstream). All column/index
  // math below is expressed in *global* terms (as if one full-width
  // pixmap were still being generated) and only converted to pixmap-
  // local coordinates at the point something is actually drawn, so a
  // tiled render is pixel-identical to what the untiled render would
  // have produced at the same position.
  //
  int total_width=d_energy.size()/(x_shrink*d_energy_channels);
  if(width<0) {
    width=total_width-start_col;
  }
  QPixmap pix(width,height);
  pix.fill(Qt::white);  // FIXME: make the background transparent
  QPainter *p=new QPainter(&pix);
  p->setFont(d_font_engine->defaultFont());

  //
  // Time Scale
  //
  if(incl_scale) {
    int interval=2*rda->system()->sampleRate()/1152;
    int end_col=start_col+width;
    int n=start_col/interval;
    if((n*interval)<start_col) {
      n++;
    }
    if(n<1) {
      n=1;
    }
    for(int global_col=n*interval;global_col<end_col;
	global_col+=interval,n++) {
      int local_col=global_col-start_col;
      int msec=2000*n;
      p->setPen(Qt::gray);
      p->drawLine(local_col,0,local_col,height);
      p->setPen(Qt::red);
      for(unsigned j=0;j<d_energy_channels;j++) {
	p->drawText(local_col+5,(j+1)*height/d_energy_channels-2,
		    RDGetTimeLength(msec*x_shrink,false,false));
      }
    }
  }

  //
  // Gain Ratio
  //
  double ratio=exp10((double)gain/2000.0);

  //
  // Waveform
  //
  p->setPen(Qt::black);
  //  int ref_line=exp10((double)(-REFERENCE_LEVEL)/2000.00)*height*ratio/
  //    ((double)d_energy_channels*2.0);
  int clip_line=height/(2*d_energy_channels);
  int start_index=start_col*x_shrink*d_energy_channels;
  int end_index=(start_col+width)*x_shrink*d_energy_channels;
  if(end_index>d_energy.size()) {
    end_index=d_energy.size();
  }
  for(unsigned i=0;i<d_energy_channels;i++) {
    int zero_line=height/(d_energy_channels*2)+i*height/(d_energy_channels);
    if(incl_scale) {
      /*
      if(ref_line<clip_line) {
	p->setPen(Qt::red);
	p->drawLine(0,zero_line+ref_line,
		    width,zero_line+ref_line);
	p->drawLine(0,zero_line-ref_line,
		    width,zero_line-ref_line);
	p->setPen(Qt::black);
      }
      */
    }
    p->drawLine(0,zero_line,width,zero_line);
    for(int j=start_index+i;j<end_index;j+=(d_energy_channels*x_shrink)) {
      uint16_t lvl=d_energy.at(j);
      for(int k=1;k<x_shrink;k++) {
	if(((j+k)<d_energy.size())&&(d_energy.at(j+k))>lvl) {
	  lvl=d_energy.at(j+k);
	}
      }
      int rlvl=(int)(ratio*(double)lvl*(double)height/
		     (65534.0*(double)d_energy_channels));
      if(rlvl>clip_line) {
	rlvl=clip_line;
      }
      int col=(j-start_index)/(x_shrink*d_energy_channels);

      // Bottom half
      p->fillRect(col,zero_line,1,rlvl,Qt::black);

      // Top half
      p->fillRect(col,zero_line,1,-rlvl,Qt::black);
    }
  }

  //
  // Dividing Line
  //
  p->setPen(Qt::gray);
  for(unsigned i=1;i<d_energy_channels;i++) {
    p->drawLine(0,i*height/d_energy_channels,
		width,i*height/d_energy_channels);
  }

  p->end();
  delete p;

  return pix;
}


bool RDWaveFactory::setCut(QString *err_msg,unsigned cartnum,int cutnum)
{
  d_energy.clear();
  d_cart_number=cartnum;
  d_cut_number=cutnum;

  //
  // Get Cut Info
  //
  RDCut *cut=new RDCut(cartnum,cutnum);
  if(!cut->exists()) {
    *err_msg=QObject::tr("No such cart/cut!");
    delete cut;
    return false;
  }
  d_channels=cut->channels();
  delete cut;
  d_energy_channels=d_channels;
  if(d_track_mode==RDWaveFactory::SingleTrack) {
    d_energy_channels=1;
  }

  //
  // Get Cut Energy Data
  //
  RDPeaksExport::ErrorCode err_code;
  RDPeaksExport *conv=new RDPeaksExport();

  conv->setCartNumber(cartnum);
  conv->setCutNumber(cutnum);
  if((err_code=conv->runExport(rda->user()->name(),rda->user()->password()))!=
     RDPeaksExport::ErrorOk) {
    *err_msg=QObject::tr("Energy export failed")+": "+
      RDPeaksExport::errorText(err_code);
    delete conv;
    return false;
  }
  if((d_track_mode==RDWaveFactory::SingleTrack)&&(d_channels==2)) { // Mix-down
    for(unsigned i=0;i<conv->energySize();i+=2) {
      uint32_t frame=
	((uint32_t)conv->energy(i)+(uint32_t)conv->energy(i+1))/2;
      d_energy.push_back(frame);
    }    
  }
  else {  // Pass-through
    for(unsigned i=0;i<conv->energySize();i++) {
      d_energy.push_back(conv->energy(i));
    }
  }
  delete conv;

  return true;
}


QList<uint16_t> RDWaveFactory::energy() const
{
  return d_energy;
}


int RDWaveFactory::energySize() const
{
  return d_energy.size()/d_energy_channels;
}


int RDWaveFactory::referenceHeight(int height,int gain)
{
  return (int)((double)height*32767.0*
	       exp10((double)(gain-REFERENCE_LEVEL)/2000.0));
}
