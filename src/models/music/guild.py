from typing import List, TYPE_CHECKING

from sqlalchemy import (
    String,
    BigInteger,
    select,
    delete
)
from sqlalchemy.orm import (
    Mapped,
    mapped_column,
    relationship
)
from sqlalchemy.dialects.postgresql import insert

from src.configs.postgres import async_session, get_async_session
from src.models import BaseModel

if TYPE_CHECKING:
    from src.models import HistoryModel


class GuildModel(BaseModel):
    __tablename__ = 'guild'

    guild_id: Mapped[int] = mapped_column(BigInteger, unique=True, index=True, nullable=False)

    channel_id: Mapped[int] = mapped_column(BigInteger, nullable=False)
    track_message_id: Mapped[int] = mapped_column(BigInteger, nullable=False)
    queue_message_id: Mapped[int] = mapped_column(BigInteger, nullable=False)
    locale: Mapped[str] = mapped_column(String(length=2), nullable=False)

    guild_history: Mapped[List['HistoryModel']] = relationship(back_populates='guild', lazy='selectin')

    @classmethod
    async def add(
        cls,
        guild_id: int,
        channel_id: int,
        track_message_id: int,
        queue_message_id: int,
        locale: str
    ) -> None:
        async with get_async_session() as session:
            query = (
                insert(cls)
                .values(
                    guild_id=guild_id,
                    channel_id=channel_id,
                    track_message_id=track_message_id,
                    queue_message_id=queue_message_id,
                    locale=locale
                )
                .on_conflict_do_update(
                    index_elements=[cls.guild_id],
                    set_=dict(
                        channel_id=channel_id,
                        track_message_id=track_message_id,
                        queue_message_id=queue_message_id,
                        locale=locale
                    )
                )
            )

            await session.execute(query)
            await session.commit()

    @classmethod
    async def get_all(cls) -> list['GuildModel']:
        async with get_async_session() as session:
            query = (
                select(cls)
            )

            setup_models = (await session.execute(query)).all()

            return setup_models

    @classmethod
    async def get(cls, guild_id: int) -> 'GuildModel':
        async with get_async_session() as session:
            query = (
                select(cls)
                .filter_by(guild_id=guild_id)
            )

            setup_model = (await session.execute(query)).one_or_none()

            return setup_model

    @classmethod
    async def delete(cls, guild_id: int) -> None:
        async with get_async_session() as session:
            query = (
                delete(cls)
                .filter_by(guild_id=guild_id)
            )

            await session.execute(query)
            await session.commit()
