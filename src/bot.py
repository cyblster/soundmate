import discord
import lavalink
from discord.ext import commands

from src.cogs import Music
from src.configs.lavalink import LavalinkClient
from src.configs.logger import Logger
from src.configs.postgres import async_engine
from src.models import BaseModel


class Bot(commands.Bot):
    def __init__(self, token: str):
        super().__init__(
            command_prefix=commands.when_mentioned_or(),
            intents=discord.Intents.all()
        )

        self.lavalink: lavalink.Client = None

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

        self.lavalink = LavalinkClient(self).lavalink

        await self.add_cog(Music(self))

        await self.tree.sync()

    def is_user_with_bot(self, user: discord.Member) -> bool:
        voice_client = user.guild.voice_client
        if not voice_client:
            return False

        if not self.is_user_connected(user):
            return False

        return user.voice.channel == voice_client.channel
