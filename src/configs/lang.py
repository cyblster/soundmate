from __future__ import annotations
from typing import Literal

from dataclasses import dataclass


Locale = Literal['ru', 'en']


@dataclass
class Language:
    def __getattr__(self, name: str) -> str:
        return '???'


class Russian(Language):
    TrackSelectPlaceholder = 'Выберите нужное'


class English(Language):
    TrackSelectPlaceholder = 'Choose variant'


@dataclass
class Emoji:
    SpeechBalloon = ':speech_balloon:'
    SkipTrack = ':next_track:'
    NoEntry = ':no_entry:'
    MagRight = ':mag_right:'
    Link = ':link:'
    Pencil = ':pencil:'
    Bookmark = ':bookmark:'
