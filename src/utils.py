from typing import Optional

import re
import requests
from datetime import (
    datetime,
    timedelta,
    timezone
)


def get_formatted_duration(duration: Optional[int]) -> str:
    if duration is None:
        return 'Неизвестно'

    dt = datetime.fromtimestamp(duration // 1000 - 1, tz=timezone.utc)
    if dt.day == 1:
        if dt.hour == 0:
            return dt.strftime('%M:%S')
        return dt.strftime('%H:%M:%S')

    dt -= timedelta(days=1)
    return dt.strftime('%d:%H:%M:%S')


def get_hq_thumbnail(uri: str) -> str:
    path = re.match(r".*/", uri).group(0)

    return f'{path}hqdefault.jpg'
