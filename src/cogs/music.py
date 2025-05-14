from typing import List, TYPE_CHECKING

import discord
import lavalink
from discord.ext import commands

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
        self.bot.loop.create_task(self.register_ui())

    async def register_ui(self):
        await self.bot.wait_until_ready()

        music_models = await GuildModel.get_all()
        for music_model in music_models:
            channel = self.bot.get_channel(music_model.channel_id)
            if channel:
                player_message = channel.get_partial_message(music_model.track_message_id)
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
                    await GuildModel.delete(music_model.guild_id)
                    raise commands.MessageNotFound('Player message not found')

                queue_message = channel.get_partial_message(music_model.queue_message_id)
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
                    await GuildModel.delete(music_model.guild_id)
                    raise commands.MessageNotFound('Player message not found')
            else:
                await GuildModel.delete(music_model.guild_id)
                raise commands.ChannelNotFound('Music channel not found')

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

        track_message = await interaction.channel.send(
            embed=NothingPlayEmbed(),
            view=NothingPlayView(self, interaction.guild)
        )

        queue_message = await interaction.channel.send(
            embed=QueueEmbed(),
            view=QueueView(interaction.guild_id)
        )

        await GuildModel.add(
            interaction.guild_id,
            interaction.channel_id,
            track_message_id=track_message.id,
            queue_message_id=queue_message.id,
            locale=language
        )

        await interaction.delete_original_response()

    async def cog_command_error(self, ctx, error) -> None:
        if isinstance(error, commands.CommandInvokeError):
            self.bot.logger.error(repr(error))

    @lavalink.listener(lavalink.TrackStartEvent)
    async def on_track_start(self, event: lavalink.TrackStartEvent) -> None: # TODO
        player: LavalinkPlayer = event.player.guild_id

        guild = self.bot.get_guild(player.guild_id)
        if not guild:
            await self.lavalink.player_manager.destroy(event.player.guild_id)

        channel = self.bot.get_channel(player.fetch('channel_id'))
        if channel:
            pass

    @lavalink.listener(lavalink.QueueEndEvent)
    async def on_queue_end(self, event: lavalink.QueueEndEvent) -> None:
        guild = self.bot.get_guild(event.player.guild_id)
        if guild:
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
    pass


class OrderTrackModal(discord.ui.Modal):
    def __init__(self, cog: Music):
        self.cog = cog

        super().__init__(title='Добавить трек в очередь', timeout=None)

        self.add_item(discord.ui.TextInput(label='Введите строку для поиска или URL'))

    async def on_submit(self, interaction: discord.Interaction):
        await interaction.response.defer(ephemeral=True, thinking=True)


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

        self.add_field(
            name='Подсказка:',
            value='Чтобы воспроизвести трек, воспользуйтесь кнопкой **"Добавить"**.'
        )

        self.colour = 15548997


class QueueEmbed(discord.Embed):
    def __init__(self, queue: List[lavalink.AudioTrack] = None):
        super().__init__(title='Очередь')

        if queue:
            for i, track in enumerate(queue[:20], 1): # TODO
                self.add_field(
                    name=f'**{i}.** {track.author}',
                    value=f'[{track.title}]({track.uri}) ({track.duration})',
                    inline=False
                )
        else:
            self.add_field(
                name='В очереди ничего нет.',
                value='Чтобы добавить трек в очередь, нажмите на кнопку **"Добавить"**.'
            )

        self.colour = 15548997


class HistoryEmbed(discord.Embed):
    def __init__(self, history: list[HistoryModel]):
        super().__init__(title='Недавнее')

        if history:
            for i, row in enumerate(history, 1):
                self.add_field(
                    name='**{}.** {}'.format(
                        i,
                        row.track_channel
                    ),
                    value='[{}]({})'.format(row.track_title, row.track_url),
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

        self.colour = 15548997
