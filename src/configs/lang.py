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
    SpeechBalloon = '💬'
    SkipTrack = '⏭️'
    NoEntry = '⛔'
    MagRight = '🔎'
    Link = '🔗'
    Pencil = '✏️'
    Bookmark = '🔖'
