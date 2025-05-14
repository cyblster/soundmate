from typing import Optional

from os import getenv
from functools import lru_cache

from pydantic import BaseSettings


@lru_cache
def get_env_filename():
    runtime_env = getenv('ENV')
    return f'.env.{runtime_env}' if runtime_env else '.env'


class EnvironmentSettings(BaseSettings):
    BOT_TITLE: Optional[str] = 'tunebot'

    PG_HOST: str
    PG_PORT: int
    PG_USER: str
    PG_PASSWORD: str
    PG_DB: str

    LL_HOST: str
    LL_PORT: int
    LL_PASSWORD: str
    LL_REGION: str

    DEBUG: bool

    class Config:
        env_file = get_env_filename()
        env_file_encoding = "utf-8"


@lru_cache
def get_environment_variables():
    return EnvironmentSettings()
