from src.bot import Bot
from src.configs.environment import get_environment_variables


env = get_environment_variables()


if __name__ == '__main__':
    Bot(env.BOT_TOKEN)
