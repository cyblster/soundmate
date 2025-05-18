from typing import TYPE_CHECKING
if TYPE_CHECKING:
    from src.bot import Bot

import discord
import lavalink

from src.configs.environment import get_environment_variables


env = get_environment_variables()


class LavalinkClient(discord.Client):
    _instance: 'LavalinkClient' = None
    _initialized: bool = False

    def __new__(cls, *args, **kwargs):
        if cls._instance is None:
            cls._instance = super().__new__(cls)

        return cls._instance

    def __init__(self, bot: 'Bot', retries: int = 5):
        if self._initialized:
            return

        super().__init__(intents=bot.intents)

        self.lavalink = lavalink.Client(bot.user.id, player=LavalinkPlayer)
        self.logger = bot.logger

        node_exception = None
        for attempt in range(retries, 0, -1):
            try:
                self.lavalink.add_node(
                    host=env.LL_HOST,
                    port=env.LL_PORT,
                    password=env.LL_PASSWORD,
                    region=env.LL_REGION,
                    name='default-node'
                )
                break
            except lavalink.ClientError as node_exception:
                pass
        else:
            raise node_exception

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
        self.logger = client.logger
        self.guild_id = voice_channel.guild.id

    async def on_voice_server_update(self, data):
        await self.lavalink.voice_update_handler({
            't': 'VOICE_SERVER_UPDATE',
            'd': data
        })

    async def on_voice_state_update(self, data):
        channel_id = data['channel_id']
        if not channel_id:
            await self._refresh()
            return

        self.channel: discord.VoiceChannel = self.client.get_channel(int(data['channel_id']))

        await self.lavalink.voice_update_handler({
            't': 'VOICE_STATE_UPDATE',
            'd': data
        })

    async def connect(
        self,
        *,
        timeout: float,
        reconnect: bool,
        self_deaf: bool = True,
        self_mute: bool = False
    ) -> None:
        player: LavalinkPlayer = self.lavalink.player_manager.create(guild_id=self.channel.guild.id)

        await self.channel.guild.change_voice_state(
            channel=self.channel,
            self_deaf=self_deaf,
            self_mute=self_mute
        )

        self.logger.debug(f'[{player.guild.name}] - Connected to the guild.')

    async def disconnect(self, *, force: bool = True) -> None:
        player: LavalinkPlayer = self.lavalink.player_manager.get(self.channel.guild.id)

        if not force and not player.is_connected:
            return

        await self.channel.guild.change_voice_state(channel=None)

        player.channel_id = None
        await self._refresh()

        self.logger.debug(f'[{player.guild.name}] - Disconnected from the guild.')

    async def _refresh(self):
        self.cleanup()

        player: LavalinkPlayer = self.lavalink.player_manager.get(self.guild_id)
        await player.stop()


class LavalinkPlayer(lavalink.DefaultPlayer):
    def __init__(self, guild_id, node):
        super().__init__(guild_id, node)

        self.bot: 'Bot' = None

        self.guild: discord.Guild = None
        self.channel: discord.TextChannel = None
        self.player_message: discord.Message = None
        self.queue_message: discord.Message = None

        self.last: lavalink.AudioTrack = None
