// rdalsacard.cpp
//
// Abstract ALSA 'card' information
//
//   (C) Copyright 2019-2025 Fred Gleason <fredg@paravelsystems.com>
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

#include "rdalsacard.h"

RDAlsaCard::RDAlsaCard(snd_ctl_t *ctl,int index)
{
  snd_ctl_card_info_t *card_info;
  snd_pcm_info_t *pcm_info;

  card_index=index;

  snd_ctl_card_info_malloc(&card_info);
  snd_pcm_info_malloc(&pcm_info);

  snd_ctl_card_info(ctl,card_info);
  card_id=QString(snd_ctl_card_info_get_id(card_info));
  card_driver=QString(snd_ctl_card_info_get_driver(card_info));
  card_name=QString(snd_ctl_card_info_get_name(card_info));
  card_long_name=QString(snd_ctl_card_info_get_longname(card_info));
  card_mixer_name=QString(snd_ctl_card_info_get_mixername(card_info));
  if(card_name=="Loopback") {  // Fix the opaque name assigned by Wheatstone
    card_name.replace("Loopback","WheatNet");
    card_long_name.replace("Loopback","WheatNet");
    card_mixer_name.replace("Loopback","WheatNet");
  }
  card_enabled=false;
  snd_pcm_info_free(pcm_info);
  snd_ctl_card_info_free(card_info);
}


int RDAlsaCard::index() const
{
  return card_index;
}


QString RDAlsaCard::id() const
{
  return card_id;
}


QString RDAlsaCard::driver() const
{
  return card_driver;
}


QString RDAlsaCard::name() const
{
  return card_name;
}


QString RDAlsaCard::longName() const
{
  return card_long_name;
}


QString RDAlsaCard::mixerName() const
{
  return card_long_name;
}


bool RDAlsaCard::isEnabled() const
{
  return card_enabled;
}


void RDAlsaCard::setEnabled(bool state)
{
  card_enabled=state;
}


QString RDAlsaCard::dump() const
{
  QString ret=QString::asprintf("Card %d\n",index());

  ret+="  ID: "+id()+"\n";
  ret+="  Name: "+name()+"\n";
  ret+="  LongName: "+longName()+"\n";
  ret+="  MixerName: "+mixerName()+"\n";

  return ret;
}
