from __future__ import annotations

import logging
from dataclasses import dataclass

from src.configs.environment import get_environment_variables


env = get_environment_variables()


class Formatter(logging.Formatter):
    def __init__(self):
        super().__init__(
            class_=logging.Formatter,
            fmt='[%(asctime)s] %(name)s |%(levelname)s| - %(message)s',
            datefmt='%Y-%m-%d %H:%M:%S',
        )

class StreamHandler(logging.StreamHandler):
    def __init__(self):
        super().__init__()

        self.setFormatter(Formatter())


@dataclass
class Log:
    level: int | str
    text: str

    @classmethod
    def format(cls, **kwargs) -> Log:
        return Log(cls.level, cls.text.format(**kwargs))


class Logger(logging.Logger):
    def __init__(self):
        self.addHandler(StreamHandler())
        self.setLevel(logging.DEBUG if env.DEBUG else logging.INFO)

    def log(self, log: Log):
        super().log(log.level, log.text)


class LogMessage:
    INIT_UI_TASK_CREATED = Log(logging.DEBUG, 'Init ui task created')
    YOUTUBE_SEARCH_FAILURE = Log(logging.ERROR, 'Youtube search failure')
    USER_ADDED_TRACK_TO_QUEUE = Log(logging.INFO, 'User added track to queue')
