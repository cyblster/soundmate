from typing import List, TYPE_CHECKING

from sqlalchemy import (
    BigInteger,
    Text,
    ForeignKey,
    select,
)
from sqlalchemy.orm import (
    Mapped,
    mapped_column,
    relationship
)
from sqlalchemy.dialects.postgresql import insert

from src.configs.postgres import get_async_session
from src.models import BaseModel

if TYPE_CHECKING:
    from src.models import GuildModel


class HistoryModel(BaseModel):
    __tablename__ = 'history'

    guild_id: Mapped[int] = mapped_column(
        BigInteger,
        ForeignKey('guild.guild_id', onupdate='CASCADE', ondelete='CASCADE'),
        index=True,
        nullable=False
    )

    track_url: Mapped[str] = mapped_column(Text, nullable=False)
    track_channel: Mapped[str] = mapped_column(Text, nullable=False)
    track_title: Mapped[str] = mapped_column(Text, nullable=False)

    guild: Mapped['GuildModel'] = relationship(back_populates='guild_history', lazy='joined')

    @classmethod
    async def add(
        cls,
        guild_id: int,
        track_channel: str,
        track_title: str,
        track_url: str
    ) -> None:
        async with get_async_session() as session:
            query = (
                insert(cls)
                .values(
                    guild_id=guild_id,
                    track_channel=track_channel,
                    track_title=track_title,
                    track_url=track_url
                )
            )

            await session.execute(query)
            await session.commit()

    @classmethod
    async def get(cls, guild_id: int) -> List['HistoryModel']:
        async with get_async_session() as session:
            query = (
                select(cls)
                .filter_by(guild_id=guild_id)
                .limit(20)
                .order_by(cls.added.desc())
            )

            history_models = (await session.execute(query)).all()

            return history_models
