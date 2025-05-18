from __future__ import annotations
from datetime import datetime

from sqlalchemy import (
    MetaData,
    Integer,
    DateTime,
    func
)
from sqlalchemy.orm import (
    DeclarativeBase,
    Mapped,
    mapped_column
)


class BaseModel(DeclarativeBase):
    __abstract__ = True

    id: Mapped[int] = mapped_column(Integer, primary_key=True, autoincrement=True)
    added: Mapped[datetime] = mapped_column(DateTime(timezone=False), server_default=func.now())
    updated: Mapped[datetime] = mapped_column(DateTime(timezone=False), nullable=True)

    metadata = MetaData(naming_convention={
        "ix": 'ix_%(column_0_label)s',
        "uq": "uq_%(table_name)s_%(column_0_name)s",
        "ck": "ck_%(table_name)s_%(constraint_name)s",
        "fk": "fk_%(table_name)s_%(column_0_name)s_%(referred_table_name)s",
        "pk": "pk_%(table_name)s"
    })


from .guild import GuildModel
from .history import HistoryModel
