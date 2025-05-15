from typing import List, TYPE_CHECKING

import discord
import lavalink
import validators
from discord.ext import commands

from src import utils
from src.configs.lavalink import (
    LavalinkClient,
    LavalinkVoiceClient,
    LavalinkPlayer
)
from src.configs.lang import (
    Locale,
    Emoji
)
from src.configs.logger import LogMessage
from src.models import *

if TYPE_CHECKING:
    from src.bot import Bot


Player = lavalink.DefaultPlayer


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
            channel = self.bot.get_channel(guild_model.channel_id)
            if channel:
                player_message = channel.get_partial_message(guild_model.player_message_id)
                if player_message:
                    await player_message.edit(
                        embed=NothingPlayEmbed(),
                        view=NothingPlayView(self, channel.guild)
                    )
                    self.bot.add_view(
                        view=NothingPlayView(self, channel.guild),
                        message_id=player_message.id
                    )
                    self.bot.add_view(
                        view=PlayNowView(self, channel.guild),
                        message_id=player_message.id
                    )
                else:
                    await GuildModel.delete(guild_model.guild_id)
                    raise commands.MessageNotFound('Player message not found')

                queue_message = channel.get_partial_message(guild_model.queue_message_id)
                if queue_message:
                    await queue_message.edit(
                        embed=QueueEmbed(),
                        view=QueueView(channel.guild.id)
                    )
                    self.bot.add_view(
                        view=QueueView(channel.guild.id),
                        message_id=queue_message.id
                    )
                else:
                    raise commands.MessageNotFound('Player message not found')

                await self.create_player(guild_model.guild_id, guild_model.channel_id)
            else:
                raise commands.ChannelNotFound('Music channel not found')

    async def create_player(self, guild_id: int, channel_id: int):
        player: LavalinkPlayer = self.lavalink.player_manager.create(guild_id)
        player.store('channel_id', channel_id)

    @discord.app_commands.command(
        name='setup',
        description='Выбрать текущий канал в качестве музыкального'
    )
    @discord.app_commands.describe(
        language='Язык интерфейса для взаимодействия'
    )
    async def command_setup(self, interaction: discord.Interaction, language: Locale) -> None:
        if not self.bot.user_is_administrator(interaction.user):
            return await interaction.response.send_message(
                f'{Emoji.NoEntry} Команда доступна только администраторам сервера.',
                ephemeral=True,
                delete_after=10
            )

        await interaction.channel.edit(**dict(
            topic=f'Канал музыкального бота {self.bot.user.name.split("#")[0]}',
            overwrites={interaction.guild.default_role: discord.PermissionOverwrite(send_messages=False)}
        ))

        await interaction.response.send_message('Производится установка...', ephemeral=True)

        player_message = await interaction.channel.send(
            embed=NothingPlayEmbed(),
            view=NothingPlayView(self, interaction.guild)
        )

        queue_message = await interaction.channel.send(
            embed=QueueEmbed(),
            view=QueueView(interaction.guild_id)
        )

        await self.create_player(interaction.guild_id, interaction.channel_id)

        await GuildModel.add(
            interaction.guild_id,
            interaction.channel_id,
            player_message.id,
            queue_message.id,
            language
        )

        await interaction.delete_original_response()

    async def cog_command_error(self, ctx, error) -> None:
        if isinstance(error, commands.CommandInvokeError):
            self.bot.logger.error(repr(error))

    @lavalink.listener(lavalink.TrackStartEvent)
    async def on_track_start(self, event: lavalink.TrackStartEvent) -> None: # TODO
        player: LavalinkPlayer = event.player

        guild = self.bot.get_guild(player.guild_id)
        if not guild:
            await self.lavalink.player_manager.destroy(event.player.guild_id)

        guild_model = await GuildModel.get(player.guild_id)

        channel = self.bot.get_channel(guild_model.channel_id)
        if channel:
            player_message = channel.get_partial_message(guild_model.player_message_id)
            if player_message:
                await player_message.edit(
                    embed=PlayNowEmbed(player.current),
                    view=PlayNowView(self, channel.guild.id)
                )
            else:
                raise commands.MessageNotFound('Player message not found')

            queue_message = channel.get_partial_message(guild_model.queue_message_id)
            if queue_message:
                await queue_message.edit(
                    embed=QueueEmbed(player.queue),
                    view=QueueView(channel.guild.id)
                )
            else:
                raise commands.MessageNotFound('Player message not found')
        else:
            raise commands.ChannelNotFound('Music channel not found')

        await HistoryModel.add(
            player.guild_id,
            player.current.author,
            player.current.title,
            player.current.uri
        )

    @lavalink.listener(lavalink.QueueEndEvent)
    async def on_queue_end(self, event: lavalink.QueueEndEvent) -> None:
        guild = self.bot.get_guild(event.player.guild_id)
        if guild and guild.voice_client:
            await guild.voice_client.disconnect(force=True)


class PlayView(discord.ui.View):
    def __init__(self):
        super().__init__(timeout=None)

    @discord.ui.button(
        custom_id='btn_add',
        emoji=Emoji.SpeechBalloon,
        label='Добавить',
        style=discord.ButtonStyle.green
    )
    async def btn_add(self, interaction: discord.Interaction, button: discord.Button):
        pass

    @discord.ui.button(
        custom_id='btn_skip',
        emoji=Emoji.SkipTrack,
        label='Пропустить',
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
        label='Отключить',
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
        label='Добавить',
        style=discord.ButtonStyle.green
    )
    async def btn_add(self, interaction: discord.Interaction, button: discord.Button):
        if not self.cog.bot.is_user_connected(interaction.user):
            return await interaction.response.send_message(
                f'{Emoji.NoEntry} Вы не подключены к голосовому каналу сервера.',
                ephemeral=True,
                delete_after=10
            )
        if self.cog.bot.is_connected_to_guild(interaction.guild_id) and not self.cog.bot.is_user_with_bot(interaction.user):
            return await interaction.response.send_message(
                f'{Emoji.NoEntry} Бот уже подключен к голосовому каналу сервера.',
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
        label='Пропустить',
        style=discord.ButtonStyle.gray
    )
    async def btn_skip(self, interaction: discord.Interaction, button: discord.Button):
        await interaction.response.defer()

        if self.cog.bot.is_user_with_bot(interaction.user):
            await self.player.skip()

    @discord.ui.button(
        custom_id='btn_disconnect',
        label='Отключить',
        style=discord.ButtonStyle.danger
    )
    async def btn_disconnect(self, interaction: discord.Interaction, button: discord.Button):
        await interaction.response.defer()

        if self.cog.bot.is_user_with_bot(interaction.user):
            voice_client = interaction.guild.voice_client
            if voice_client:
                await voice_client.disconnect(force=True)


class OrderTrackModal(discord.ui.Modal):
    def __init__(self, cog: Music):
        super().__init__(title='Добавить трек в очередь', timeout=None)

        self.cog = cog

        self.add_item(discord.ui.TextInput(label='Введите строку для поиска или URL'))

    async def on_submit(self, interaction: discord.Interaction):
        await interaction.response.defer(ephemeral=True, thinking=True)

        player: LavalinkPlayer = self.cog.lavalink.player_manager.get(interaction.guild_id)

        query = self.children[0].value
        if validators.url(query):
            search_result = await player.node.get_tracks(query)
            if search_result.load_type == lavalink.LoadType.EMPTY:
                await interaction.delete_original_response()
                raise commands.ObjectNotFound('URL request not found')
            if search_result.load_type == lavalink.LoadType.ERROR:
                await interaction.delete_original_response()
                raise commands.ObjectNotFound('Outdated YouTube plugin signature')
            if search_result.load_type == lavalink.LoadType.PLAYLIST:
                for track in search_result.tracks:
                    player.add(track, requester=interaction.user.nick)
            else:
                player.add(search_result.tracks[0], requester=interaction.user.nick)

            if not player.is_playing:
                await interaction.user.voice.channel.connect(cls=LavalinkVoiceClient)
                await player.play()

            await interaction.delete_original_response()
        else:
            search_result = await player.node.get_tracks(f'ytsearch:{query}')

            await interaction.followup.send(
                embed=TrackSelectEmbed(search_result.tracks[:5]),
                view=TrackSelectView(self.cog, interaction, search_result.tracks[:5]),
                ephemeral=True
            )


class TrackSelectView(discord.ui.View):
    def __init__(self, cog: Music, interaction: discord.Interaction, tracks: List[lavalink.AudioTrack]):
        self.interaction = interaction

        super().__init__(timeout=60)

        self.add_item(TrackSelect(cog, interaction, tracks))

    async def on_timeout(self) -> None:
        await self.interaction.delete_original_response()


class TrackSelect(discord.ui.Select):
    def __init__(self, cog: Music, interaction: discord.Interaction, tracks: List[lavalink.AudioTrack]):
        self.cog = cog
        self.interaction = interaction
        self.tracks = tracks

        super().__init__(placeholder='Выберите нужное', options=[
            discord.SelectOption(
                label=f'{i + 1}. {track.author} - {track.title}'[:100],
                value=str(i)
            )
            for i, track in enumerate(self.tracks)
        ])

    async def callback(self, interaction: discord.Interaction) -> None:
        await self.interaction.delete_original_response()
        await interaction.response.defer(ephemeral=True, thinking=True)

        self.player: LavalinkPlayer = self.cog.lavalink.player_manager.get(interaction.guild_id)

        self.player.add(self.tracks[int(self.values[0])], requester=interaction.user.nick)

        if not self.player.is_playing:
            await interaction.user.voice.channel.connect(cls=LavalinkVoiceClient)
            await self.player.play()

        await interaction.delete_original_response()


class QueueView(discord.ui.View):
    def __init__(self, guild_id: int):
        super().__init__(timeout=None)

        self.guild_id = guild_id

    @discord.ui.button(
        custom_id='btn_history',
        emoji=Emoji.Bookmark,
        label='Недавнее',
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
        super().__init__(title='Сейчас ничего не играет')

        self.colour = 15548997

        self.add_field(
            name='Подсказка:',
            value='Чтобы воспроизвести трек, воспользуйтесь кнопкой **"Добавить"**.'
        )


class PlayNowEmbed(discord.Embed):
    def __init__(self, track: lavalink.AudioTrack):
        super().__init__(title=track.title, url=track.uri)

        self.colour = 15548997

        self.set_author(name=track.author)
        self.set_image(url=track.uri)

        self.add_field(
            name='Запрошено пользователем :',
            value=f'`{track.requester}`',
            inline=True
        )
        self.add_field(
            name='Длительность :',
            value=f'`{utils.get_formatted_duration(track.duration)}`',
            inline=True
        )

        self.set_footer(text='YouTube')


class TrackSelectEmbed(discord.Embed):
    def __init__(self, tracks: List[lavalink.AudioTrack]):
        super().__init__(title='Результаты поиска :')

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
        super().__init__(title='Очередь')

        self.colour = 15548997

        if queue:
            for i, track in enumerate(queue[:20], 1): # TODO
                self.add_field(
                    name=f'**{i}.** {track.author}',
                    value=f'[{track.title}]({track.uri}) '
                          f'({utils.get_formatted_duration(track.duration)})',
                    inline=False
                )
        else:
            self.add_field(
                name='В очереди ничего нет.',
                value='Чтобы добавить трек в очередь, нажмите на кнопку **"Добавить"**.'
            )


class HistoryEmbed(discord.Embed):
    def __init__(self, history: list[HistoryModel]):
        super().__init__(title='Недавнее')

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
                value='Чтобы добавить трек в очередь, нажмите на кнопку **"Добавить"**.'
            )
        else:
            self.add_field(
                name='В истории ничего нет.',
                value='Чтобы добавить трек в очередь, нажмите на кнопку **"Добавить"**.'
            )
