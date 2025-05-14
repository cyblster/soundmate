import discord
from discord.ext import commands

from cogs import Music
from configs.lavalink import LavalinkClient
from configs.logger import Logger
from configs.postgres import async_engine
from models import BaseModel


class Bot(commands.Bot):
    def __init__(self, token: str):
        super().__init__(
            command_prefix=commands.when_mentioned_or(),
            intents=discord.Intents.all()
        )

        self.lavalink = LavalinkClient(self).lavalink

        self.logger = Logger()
        self.logger_handler = self.logger.handlers[0]

        self.run(
            token,
            log_handler=self.logger_handler,
            log_formatter=self.logger_handler.formatter
        )

    @staticmethod
    async def init_db():
        async with async_engine.begin() as connection:
            await connection.run_sync(BaseModel.metadata.create_all)

    @staticmethod
    def user_is_administrator(user: discord.Member) -> bool:
        return user.guild_permissions.administrator

    @staticmethod
    def is_user_connected(user: discord.Member) -> bool:
        return user.voice is not None

    async def on_connect(self) -> None:
        await self.init_db()

    async def on_ready(self) -> None:
        await self.wait_until_ready()

        await self.add_cog(Music(self))

        await self.tree.sync()

    def is_connected_to_guild(self, guild_id) -> bool:
        return self.lavalink.player_manager.get(guild_id) is not None

    def is_user_with_bot(self, user: discord.Member) -> bool:
        return user.voice.channel in [vc.channel for vc in self.voice_clients]
