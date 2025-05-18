import logging
from dataclasses import dataclass

from src.configs.environment import get_environment_variables


env = get_environment_variables()


class Formatter(logging.Formatter):
    def __init__(self):
        super().__init__(
            fmt='[%(asctime)s] %(name)s |%(levelname)s| - %(message)s',
            datefmt='%Y-%m-%d %H:%M:%S',
        )


class StreamHandler(logging.StreamHandler):
    def __init__(self):
        super().__init__()

        self.setFormatter(Formatter())


@dataclass
class LogLevel:
    CRITICAL = 50
    FATAL = CRITICAL
    ERROR = 40
    WARNING = 30
    WARN = WARNING
    INFO = 20
    DEBUG = 10
    NOTSET = 0


class LogMessage:
    def __init__(self, level: LogLevel, message: str):
        self.level = level
        self.message = message


class Logger(logging.Logger):
    def __init__(self):
        super().__init__(name=env.BOT_TITLE)

        self.addHandler(StreamHandler())
        self.setLevel(logging.DEBUG if env.DEBUG else logging.INFO)
