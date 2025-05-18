from typing import Union, Literal, get_args

from functools import lru_cache
from dataclasses import dataclass

from src.configs.environment import get_environment_variables


env = get_environment_variables()


@dataclass
class Emoji:
    SpeechBalloon = '💬'
    SkipTrack = '⏭️'
    NoEntry = '⛔'
    MagRight = '🔎'
    Link = '🔗'
    Pencil = '✏️'
    Bookmark = '🔖'


@dataclass
class Language:
    def __getattr__(self, name: str) -> str:
        return '???'


class Russian(Language):
    SetupCommandDescription = 'Выбрать текущий канал в качестве музыкального'
    SetupCommandPermissionNotify = '{emoji} Команда доступна только администраторам сервера.'
    SetupCommandChannelTopic = 'Канал музыкального бота {name}'
    SetupCommandProgressInfo = 'Производится установка...'

    PlayerButtonAdd = 'Добавить'
    PlayerButtonSkip = 'Пропустить'
    PlayerButtonDisconnect = 'Отключить'

    UserNotConnected = '{emoji} Вы не подключены к голосовому каналу сервера.'
    BotAlreadyConnected = '{emoji} Бот уже подключен к голосовому каналу сервера.'

    OrderTrackModalTitle = 'Добавить трек в очередь'
    OrderTrackModalQueryLabel = 'Введите поисковой запрос или URL'

    OrderTrackModalResultEmptyLabel = '{emoji} По вашему запросу ничего не найдено.'

    TrackSelectPlaceholder = 'Выберите нужное'

    QueueButtonHistory = 'Недавнее'

    NothingPlayEmbedTitle = 'Сейчас ничего не играет'
    NothingPlayEmbedHintFieldName = 'Подсказка:'
    NothingPlayEmbedHintFieldValue = 'Чтобы воспроизвести трек, воспользуйтесь кнопкой **"Добавить"**'

    PlayEmbedRequesterFieldName = 'Запрошено пользователем :'
    PlayEmbedDurationFieldName = 'Длительность :'

    TrackSelectEmbedTitle = 'Результаты поиска :'

    QueueEmbedTitle = 'Очередь'
    QueueEmbedHintFieldName = 'В очереди ничего нет.'
    QueueEmbedHintFieldValue = 'Чтобы добавить трек в очередь, нажмите на кнопку **"Добавить"**'
    QueueEmbedOverflowFieldName = 'И еще несколько треков ({count})'
    QueueEmbedOverflowFieldValue = QueueEmbedHintFieldValue

    HistoryEmbedTitle = 'Недавнее'
    HistoryEmbedHintFieldName = 'В истории ничего нет.'
    HistoryEmbedHintFieldValue = QueueEmbedHintFieldValue


class English(Language):
    SetupCommandDescription = 'Set the current channel as the music one'
    SetupCommandPermissionNotify = '{emoji} Command available to server admins only.'
    SetupCommandChannelTopic = 'Music channel of {name}'
    SetupCommandProgressInfo = 'Setting up...'

    PlayerButtonAdd = 'Add'
    PlayerButtonSkip = 'Skip'
    PlayerButtonDisconnect = 'Disconnect'

    UserNotConnected = '{emoji} You are not connected to a voice channel.'
    BotAlreadyConnected = '{emoji} Bot is already in a voice channel.'

    OrderTrackModalTitle = 'Add a track to queue'
    OrderTrackModalQueryLabel = 'Enter a search term or URL'

    OrderTrackModalResultEmptyLabel = '{emoji} Nothing found for your query.'

    TrackSelectPlaceholder = 'Pick one'

    QueueButtonHistory = 'History'

    NothingPlayEmbedTitle = 'Nothing is playing now'
    NothingPlayEmbedHintFieldName = 'Tip:'
    NothingPlayEmbedHintFieldValue = 'Press **“Add”** to play a track'

    PlayEmbedRequesterFieldName = 'Requested by:'
    PlayEmbedDurationFieldName = 'Length:'

    TrackSelectEmbedTitle = 'Search results:'

    QueueEmbedTitle = 'Queue'
    QueueEmbedHintFieldName = 'Queue is empty.'
    QueueEmbedHintFieldValue = 'Press **“Add”** to enqueue a track'
    QueueEmbedOverflowFieldName = 'And a few more tracks ({count})'
    QueueEmbedOverflowFieldValue = QueueEmbedHintFieldValue

    HistoryEmbedTitle = 'History'
    HistoryEmbedHintFieldName = 'History is empty.'
    HistoryEmbedHintFieldValue = QueueEmbedHintFieldValue


SupportedLocale = Literal[
    'en',
    'ru'
]
SupportedLanguage = Union[
    English,
    Russian
]


@dataclass
class LanguageManager:
    en = English
    ru = Russian

    @classmethod
    def get(cls, locale: SupportedLocale) -> SupportedLanguage:
        try:
            return getattr(cls, locale)()
        except AttributeError:
            raise UnsupportedLocaleError(locale)


class UnsupportedLocaleError(AttributeError):
    def __init__(self, locale: str):
        super().__init__(
            f'Unsupported locale: {locale}. '
            f'Supported locales are: {", ".join(get_args(SupportedLocale))}'
        )


@lru_cache
def get_application_language() -> SupportedLanguage:
    return LanguageManager.get(env.LOCALE)
