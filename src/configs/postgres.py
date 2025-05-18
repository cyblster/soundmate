from typing import AsyncGenerator

from sqlalchemy.ext.asyncio import (
    AsyncSession,
    create_async_engine,
    async_sessionmaker
)
from contextlib import asynccontextmanager

from src.configs.environment import get_environment_variables


env = get_environment_variables()


URI = f'postgresql+asyncpg://{env.PG_USER}:{env.PG_PASSWORD}@' \
      f'{env.PG_HOST}:{env.PG_PORT}/{env.PG_DB}'


async_engine = create_async_engine(URI, echo=env.DEBUG, future=True)
async_session = async_sessionmaker(autocommit=False, bind=async_engine,
                                   expire_on_commit=False)


@asynccontextmanager
async def get_async_session() -> AsyncGenerator[AsyncSession, None]:
    async with async_session() as session:
        yield session
