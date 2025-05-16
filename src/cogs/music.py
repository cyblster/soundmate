from typing import List, TYPE_CHECKING
if TYPE_CHECKING:
    from src.bot import Bot

import discord
import lavalink
import validators
from discord.ext import commands

from src import utils
from src.configs.environment import get_environment_variables
from src.configs.lavalink import (
    LavalinkClient,
    LavalinkVoiceClient,
    LavalinkPlayer
)
from src.configs.language import Emoji, get_application_language
from src.configs.logger import LogMessage
from src.models import *


env = get_environment_variables()
lang = get_application_language()


class Music(commands.Cog):
    def __init__(self, bot: 'Bot'):
        super().__init__()

        self.bot = bot
        self.lavalink = self.bot.lavalink

        self.lavalink.add_event_hooks(self)

        self.bot.loop.create_task(self.do_when_ready())

    async def do_when_ready(self):
        await self.bot.wait_until_ready()

        guild_models = await GuildModel.get_all()
        for guild_model in guild_models:
            player: LavalinkPlayer = await self.create_player(guild_model)

            player_message = await NothingPlayEmbed.update(self, player)
            self.bot.add_view(
                view=NothingPlayView(self, player.guild_id),
                message_id=player_message.id
            )
            self.bot.add_view(
                view=PlayNowView(self, player.guild_id),
                message_id=player_message.id
            )

            queue_message = await QueueEmbed.update(self, player)
            self.bot.add_view(
                view=QueueView(player.guild_id),
                message_id=queue_message.id
            )

    async def create_player(self, guild_model: GuildModel) -> LavalinkPlayer:
        channel = self.bot.get_channel(guild_model.channel_id)
        if channel:
            player_message = channel.get_partial_message(guild_model.player_message_id)
            if not player_message:
                raise PlayerMessageNotFound(self.bot, guild_model.guild_id)

            queue_message = channel.get_partial_message(guild_model.queue_message_id)
            if not player_message:
                raise QueueMessageNotFound(self.bot, guild_model.guild_id)
        else:
            raise PlayerChannelNotFound(self.bot, guild_model.guild_id)

        player: LavalinkPlayer = self.lavalink.player_manager.create(guild_model.guild_id)

        player.store('guild', channel.guild)
        player.store('channel', channel)
        player.store('player_message', player_message)
        player.store('queue_message', queue_message)

        return player

    async def add_to_queue(
        self,
        guild_id: int,
        voice_channel: discord.VoiceChannel,
        tracks: List[lavalink.AudioTrack],
        requester: str
    ):
        player: LavalinkPlayer = self.lavalink.player_manager.get(guild_id)

        for track in tracks:
            player.add(track, requester=requester)

        if not player.is_playing:
            await voice_channel.connect(cls=LavalinkVoiceClient)
            await player.play()

        if player.queue:
            await QueueEmbed.update(self, player)

    @discord.app_commands.command(
        name='setup',
        description=lang.SetupCommandDescription
    )
    async def command_setup(self, interaction: discord.Interaction) -> None:
        if not self.bot.user_is_administrator(interaction.user):
            return await interaction.response.send_message(
                lang.SetupCommandPermissionNotify.format(emoji=Emoji.NoEntry),
                ephemeral=True,
                delete_after=10
            )

        await interaction.channel.edit(
            topic=lang.SetupCommandChannelTopic.format(name=self.bot.user.name.split("#")[0]),
            overwrites={interaction.guild.default_role: discord.PermissionOverwrite(send_messages=False)}
        )

        await interaction.response.send_message(lang.SetupCommandProgressInfo, ephemeral=True)

        player_message = await interaction.channel.send(
            embed=NothingPlayEmbed(),
            view=NothingPlayView(self, interaction.guild)
        )

        queue_message = await interaction.channel.send(
            embed=QueueEmbed(),
            view=QueueView(interaction.guild_id)
        )

        guild_model = await GuildModel.add(
            interaction.guild_id,
            interaction.channel_id,
            player_message.id,
            queue_message.id
        )

        await self.create_player(guild_model)

        await interaction.delete_original_response()

    async def cog_command_error(self, ctx, error) -> None:
        if isinstance(error, commands.CommandInvokeError):
            self.bot.logger.error(repr(error))

    @commands.Cog.listener()
    async def on_guild_remove(self, guild: discord.Guild) -> None:
        await self.lavalink.player_manager.destroy(guild.id)
        await GuildModel.delete(guild.id)

    @commands.Cog.listener()
    async def on_voice_state_update(
        self,
        user: discord.Member,
        before: discord.VoiceState,
        after: discord.VoiceState
    ) -> None:
        if user == self.bot.user:
            if not after.channel:
                player: LavalinkPlayer = self.lavalink.player_manager.get(before.channel.guild.id)

                player.queue.clear()

                await NothingPlayEmbed.update(self, player)
                await QueueEmbed.update(self, player)
        elif before.channel:
            voice_client = before.channel.guild.voice_client
            if voice_client and before.channel == voice_client.channel:
                if len(before.channel.members) == 1 and before.channel.members[0] == self.bot.user:
                    await voice_client.disconnect(force=True)

    @lavalink.listener(lavalink.TrackStartEvent)
    async def on_track_start(self, event: lavalink.TrackStartEvent) -> None:
        player: LavalinkPlayer = event.player

        await PlayNowEmbed.update(self, player)
        await QueueEmbed.update(self, player)

        await HistoryModel.add(
            player.guild_id,
            player.current.author,
            player.current.title,
            player.current.uri
        )

    @lavalink.listener(lavalink.QueueEndEvent)
    async def on_queue_end(self, event: lavalink.QueueEndEvent) -> None:
        player: LavalinkPlayer = event.player

        guild: discord.Guild = player.fetch('guild')
        if guild:
            voice_client = guild.voice_client
            if voice_client:
                await voice_client.disconnect(force=True)
        else:
            raise PlayerChannelNotFound(self.bot, player.guild_id)


class PlayView(discord.ui.View):
    def __init__(self):
        super().__init__(timeout=None)

    @discord.ui.button(
        custom_id='btn_add',
        emoji=Emoji.SpeechBalloon,
        label=lang.PlayerButtonAdd,
        style=discord.ButtonStyle.green
    )
    async def btn_add(self, interaction: discord.Interaction, button: discord.Button):
        pass

    @discord.ui.button(
        custom_id='btn_skip',
        emoji=Emoji.SkipTrack,
        label=lang.PlayerButtonSkip,
        style=discord.ButtonStyle.gray,
        disabled=True
    )
    async def btn_skip(self, interaction: discord.Interaction, button: discord.Button):
        pass

    @discord.ui.button(
        custom_id='btn_stub',
        label='\u200b',
        style=discord.ButtonStyle.gray,
        disabled=True
    )
    async def btn_stub(self, interaction: discord.Interaction, button: discord.Button):
        pass

    @discord.ui.button(
        custom_id='btn_disconnect',
        label=lang.PlayerButtonDisconnect,
        style=discord.ButtonStyle.danger,
        disabled=True
    )
    async def btn_disconnect(self, interaction: discord.Interaction, button: discord.Button):
        pass


class NothingPlayView(PlayView):
    def __init__(self, cog: Music, guild_id: int):
        super().__init__()

        self.cog = cog
        self.guild_id = guild_id

    @discord.ui.button(
        custom_id='btn_add',
        emoji=Emoji.SpeechBalloon,
        label=lang.PlayerButtonAdd,
        style=discord.ButtonStyle.green
    )
    async def btn_add(self, interaction: discord.Interaction, button: discord.Button):
        if not self.cog.bot.is_user_connected(interaction.user):
            return await interaction.response.send_message(
                lang.UserNotConnected.format(emoji=Emoji.NoEntry),
                ephemeral=True,
                delete_after=10
            )
        if self.cog.bot.is_connected_to_guild(interaction.guild_id) and not self.cog.bot.is_user_with_bot(interaction.user):
            return await interaction.response.send_message(
                lang.BotAlreadyConnected.format(emoji=Emoji.NoEntry),
                ephemeral=True,
                delete_after=10
            )

        await interaction.response.send_modal(OrderTrackModal(self.cog))


class PlayNowView(NothingPlayView):
    def __init__(self, cog: Music, guild_id: int):
        super().__init__(cog, guild_id)

        self.cog = cog
        self.guild_id = guild_id

        self.player: LavalinkPlayer = self.cog.lavalink.player_manager.get(guild_id)

    @discord.ui.button(
        custom_id='btn_skip',
        emoji=Emoji.SkipTrack,
        label=lang.PlayerButtonSkip,
        style=discord.ButtonStyle.gray
    )
    async def btn_skip(self, interaction: discord.Interaction, button: discord.Button):
        if self.cog.bot.is_user_with_bot(interaction.user):
            await self.player.skip()

        await interaction.response.defer()

    @discord.ui.button(
        custom_id='btn_disconnect',
        label=lang.PlayerButtonDisconnect,
        style=discord.ButtonStyle.danger
    )
    async def btn_disconnect(self, interaction: discord.Interaction, button: discord.Button):
        if self.cog.bot.is_user_with_bot(interaction.user):
            voice_client = interaction.guild.voice_client
            if voice_client:
                await voice_client.disconnect(force=True)

        await interaction.response.defer()


class OrderTrackModal(discord.ui.Modal):
    def __init__(self, cog: Music):
        super().__init__(title=lang.OrderTrackModalTitle, timeout=None)

        self.cog = cog

        self.add_item(discord.ui.TextInput(label=lang.OrderTrackModalQueryLabel))

    async def on_submit(self, interaction: discord.Interaction):
        player: LavalinkPlayer = self.cog.lavalink.player_manager.get(interaction.guild_id)

        query = self.children[0].value
        if validators.url(query):
            search_result = await player.node.get_tracks(query)
            if search_result.load_type == lavalink.LoadType.EMPTY:
                raise PlayerRequestNotFound(self.cog.bot, interaction)
            if search_result.load_type == lavalink.LoadType.ERROR:
                raise PlayerYTSignatureError(self.cog.bot, interaction)

            await interaction.response.defer()

            await self.cog.add_to_queue(
                interaction.guild_id,
                interaction.user.voice.channel,
                search_result.tracks,
                interaction.user.nick
            )
        else:
            search_result = await player.node.get_tracks(f'ytsearch:{query}')
            if search_result.load_type == lavalink.LoadType.EMPTY:
                raise PlayerRequestNotFound(self.cog.bot, interaction)
            if search_result.load_type == lavalink.LoadType.ERROR:
                raise PlayerYTSignatureError(self.cog.bot, interaction)

            await interaction.response.send_message(
                embed=TrackSelectEmbed(search_result.tracks[:5]),
                view=TrackSelectView(self.cog, interaction, search_result.tracks[:5]),
                ephemeral=True,
                delete_after=30
            )


class TrackSelectView(discord.ui.View):
    def __init__(self, cog: Music, interaction: discord.Interaction, tracks: List[lavalink.AudioTrack]):
        self.interaction = interaction

        super().__init__(timeout=None)

        self.add_item(TrackSelect(cog, interaction, tracks))


class TrackSelect(discord.ui.Select):
    def __init__(
        self,
        cog: Music,
        interaction: discord.Interaction,
        tracks: List[lavalink.AudioTrack]
    ):
        self.cog = cog
        self.interaction = interaction
        self.tracks = tracks

        super().__init__(placeholder=lang.TrackSelectPlaceholder, options=[
            discord.SelectOption(
                label=f'{i + 1}. {track.author} - {track.title}'[:100],
                value=str(i)
            )
            for i, track in enumerate(self.tracks)
        ])

    async def callback(self, interaction: discord.Interaction) -> None:
        await self.interaction.delete_original_response()

        self.player: LavalinkPlayer = self.cog.lavalink.player_manager.get(interaction.guild_id)

        await self.cog.add_to_queue(
            interaction.guild_id,
            interaction.user.voice.channel,
            [self.tracks[int(self.values[0])]],
            requester=interaction.user.nick
        )

        await interaction.response.defer()


class QueueView(discord.ui.View):
    def __init__(self, guild_id: int):
        super().__init__(timeout=None)

        self.guild_id = guild_id

    @discord.ui.button(
        custom_id='btn_history',
        emoji=Emoji.Bookmark,
        label=lang.QueueButtonHistory,
        style=discord.ButtonStyle.gray
    )
    async def btn_history(self, interaction: discord.Interaction, button: discord.Button):
        history_models = await HistoryModel.get(self.guild_id)

        await interaction.response.send_message(
            embed=HistoryEmbed(history_models),
            ephemeral=True,
            delete_after=60
        )


class NothingPlayEmbed(discord.Embed):
    def __init__(self):
        super().__init__(title=lang.NothingPlayEmbedTitle)

        self.colour = 15548997

        self.add_field(
            name=lang.NothingPlayEmbedHintFieldName,
            value=lang.NothingPlayEmbedHintFieldValue
        )

    @staticmethod
    async def update(cog: Music, player: LavalinkPlayer) -> discord.Message:
        channel: discord.VoiceChannel = player.fetch('channel')
        if channel:
            player_message: discord.Message = player.fetch('player_message')
            if player_message:
                try:
                    await player_message.edit(
                        embed=NothingPlayEmbed(),
                        view=NothingPlayView(cog, player.guild_id)
                    )
                except discord.errors.NotFound:
                    raise PlayerMessageNotFound(cog.bot, player.guild_id)
            else:
                raise PlayerMessageNotFound(cog.bot, player.guild_id)
        else:
            raise PlayerChannelNotFound(cog.bot, player.guild_id)

        return player_message


class PlayNowEmbed(discord.Embed):
    def __init__(self, track: lavalink.AudioTrack):
        super().__init__(title=track.title, url=track.uri)

        self.colour = 15548997

        self.set_author(name=track.author)
        self.set_image(url=utils.get_hq_thumbnail(track.artwork_url))

        self.add_field(
            name=lang.PlayEmbedRequesterFieldName,
            value=f'`{track.requester}`',
            inline=True
        )
        self.add_field(
            name=lang.PlayEmbedDurationFieldName,
            value=f'`{utils.get_formatted_duration(track.duration)}`',
            inline=True
        )

        self.set_footer(text='YouTube')

    @staticmethod
    async def update(cog: Music, player: LavalinkPlayer) -> discord.Message:
        channel: discord.VoiceChannel = player.fetch('channel')
        if channel:
            player_message: discord.Message = player.fetch('player_message')
            if player_message:
                try:
                    await player_message.edit(
                        embed=PlayNowEmbed(player.current),
                        view=PlayNowView(cog, player.guild_id)
                    )
                except discord.errors.NotFound:
                    raise PlayerMessageNotFound(cog.bot, player.guild_id)
            else:
                raise PlayerMessageNotFound(cog.bot, player.guild_id)
        else:
            raise PlayerChannelNotFound(cog.bot, player.guild_id)

        return player_message


class TrackSelectEmbed(discord.Embed):
    def __init__(self, tracks: List[lavalink.AudioTrack]):
        super().__init__(title=lang.TrackSelectEmbedTitle)

        for i, track in enumerate(tracks, 1):
            self.add_field(
                name='\u200b',
                value=f'**{i}.** {track.author} - [{track.title}]({track.uri}) '
                      f'({utils.get_formatted_duration(track.duration)})',
                inline=False
            )

        self.set_footer(text='YouTube')


class QueueEmbed(discord.Embed):
    def __init__(self, queue: List[lavalink.AudioTrack] = None):
        super().__init__(title=lang.QueueEmbedTitle)

        self.colour = 15548997

        if queue:
            for i, track in enumerate(queue[:20], 1):
                self.add_field(
                    name=f'**{i}.** {track.author}',
                    value=f'[{track.title}]({track.uri}) '
                          f'({utils.get_formatted_duration(track.duration)})',
                    inline=False
                )
            if len(queue) > 20:
                self.add_field(
                    name=lang.QueueEmbedOverflowFieldName.format(count=len(queue) - 20),
                    value=lang.QueueEmbedOverflowFieldValue
                )
            else:
                self.add_field(
                    name='\u200b',
                    value=lang.QueueEmbedHintFieldValue
                )
        else:
            self.add_field(
                name=lang.QueueEmbedHintFieldName,
                value=lang.QueueEmbedHintFieldValue
            )

    @staticmethod
    async def update(cog: Music, player: LavalinkPlayer) -> discord.Message:
        channel: discord.VoiceChannel = player.fetch('channel')
        if channel:
            queue_message: discord.Message = player.fetch('queue_message')
            if queue_message:
                try:
                    await queue_message.edit(
                        embed=QueueEmbed(player.queue),
                        view=QueueView(player.guild_id)
                    )
                except discord.errors.NotFound:
                    raise PlayerMessageNotFound(cog.bot, player.guild_id)
            else:
                raise PlayerMessageNotFound(cog.bot, player.guild_id)
        else:
            raise PlayerChannelNotFound(cog.bot, player.guild_id)

        return queue_message


class HistoryEmbed(discord.Embed):
    def __init__(self, history: list[HistoryModel]):
        super().__init__(title=lang.HistoryEmbedTitle)

        self.colour = 15548997

        if history:
            for i, row in enumerate(history, 1):
                self.add_field(
                    name=f'**{i}.** {row.author}',
                    value=f'[{row.title}]({row.uri})',
                    inline=False
                )
            self.add_field(
                name='\u200b',
                value=lang.HistoryEmbedHintFieldValue
            )
        else:
            self.add_field(
                name=lang.HistoryEmbedHintFieldName,
                value=lang.HistoryEmbedHintFieldValue
            )


class PlayerEntityNotFound(commands.CommandError):
    def __init__(self, bot: 'Bot', guild_id: int, *, message: str):
        super().__init__(message)

        bot.logger.error(message)

        bot.loop.create_task(GuildModel.delete(guild_id))


class PlayerChannelNotFound(PlayerEntityNotFound):
    def __init__(self, bot: 'Bot', guild_id: int):
        super().__init__(bot, guild_id, message='Player channel not found')


class PlayerMessageNotFound(PlayerEntityNotFound):
    def __init__(self, bot: 'Bot', guild_id: int):
        super().__init__(bot, guild_id, message='Player message not found')


class QueueMessageNotFound(PlayerEntityNotFound):
    def __init__(self, bot: 'Bot', guild_id: int):
        super().__init__(bot, guild_id, message='Queue message not found')


class PlayerRequestNotFound(commands.CommandError):
    def __init__(self, bot: 'Bot', interaction: discord.Interaction):
        super().__init__(message='Tracks not found')

        bot.loop.create_task(interaction.response.send_message(
            lang.OrderTrackModalResultEmptyLabel.format(emoji=Emoji.MagRight),
            ephemeral=True,
            delete_after=10
        ))


class PlayerYTSignatureError(commands.CommandError):
    def __init__(self, bot: 'Bot', interaction: discord.Interaction):
        super().__init__(message='Outdated YouTube plugin signature')
