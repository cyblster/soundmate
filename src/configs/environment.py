from functools import lru_cache
from pydantic_settings import BaseSettings


class EnvironmentSettings(BaseSettings):
    BOT_TOKEN: str
    BOT_TITLE: str = 'soundmate'
    BOT_VERSION: str = '1.0.0'

    PG_HOST: str
    PG_PORT: int = 5432
    PG_USER: str
    PG_PASSWORD: str
    PG_DB: str

    LL_HOST: str
    LL_PORT: int = 2333
    LL_PASSWORD: str
    LL_REGION: str
    LL_OPTIONS: str = '-Xmx512M'

    LOCALE: str = 'en'

    DEBUG: bool = False

    class Config:
        env_file = '.env'
        env_file_encoding = "utf-8"


@lru_cache
def get_environment_variables():
    return EnvironmentSettings()
