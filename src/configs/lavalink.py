from typing import TYPE_CHECKING

import discord
import lavalink
from lavalink.errors import ClientError

from src.configs.environment import get_environment_variables

if TYPE_CHECKING:
    from src.bot import Bot


env = get_environment_variables()


class LavalinkClient(discord.Client):
    _instance: 'LavalinkClient' = None
    _initialized: bool = False

    def __new__(cls, *args, **kwargs):
        if cls._instance is None:
            cls._instance = super().__new__(cls)

        return cls._instance

    def __init__(self, bot: 'Bot'):
        if self._initialized:
            return

        super().__init__(intents=bot.intents)

        self.lavalink = lavalink.Client(bot.user.id)

        self.lavalink.add_node(
            host=env.LL_HOST,
            port=env.LL_PORT,
            password=env.LL_PASSWORD,
            region=env.LL_REGION,
            name='default-node'
        )

        self._initialized = True

    @property
    def node(self) -> lavalink.Node:
        return self.lavalink.node_manager.nodes[0]


class LavalinkVoiceClient(discord.VoiceProtocol):
    def __init__(
        self,
        client: LavalinkClient,
        voice_channel: discord.VoiceChannel
    ):
        super().__init__(client, voice_channel)

        self.lavalink = client.lavalink
        self.guild_id = voice_channel.guild.id
        self._destroyed = False

    async def on_voice_server_update(self, data):
        await self.lavalink.voice_update_handler({
            't': 'VOICE_SERVER_UPDATE',
            'd': data
        })

    async def on_voice_state_update(self, data):
        channel_id = data['channel_id']
        if not channel_id:
            await self._destroy()
            return

        self.channel: discord.VoiceChannel = self.client.get_channel(int(channel_id))

        await self.lavalink.voice_update_handler({
            't': 'VOICE_STATE_UPDATE',
            'd': data
        })

    async def connect(self, *args, **kwargs) -> None:
        if not self.channel:
            return

        player = self.lavalink.player_manager.get(self.channel.guild.id)
        if not player:
            self.lavalink.player_manager.create(guild_id=self.channel.guild.id)

        await self.channel.guild.change_voice_state(
            channel=self.channel,
            self_mute=False,
            self_deaf=True
        )

    async def disconnect(self, *args, **kwargs) -> None:
        if self.channel:
            player = self.lavalink.player_manager.get(self.channel.guild.id)
            if player:
                player.channel_id = None

            await self.channel.guild.change_voice_state(channel=None)

        await self._destroy()

    async def _destroy(self):
        self.cleanup()

        if self._destroyed:
            return

        self._destroyed = True

        try:
            await self.lavalink.player_manager.destroy(self.guild_id)
        except ClientError:
            pass # TODO send logs


class LavalinkPlayer(lavalink.DefaultPlayer):
    pass
