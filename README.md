# streaming-service

Модуль стриминга видео для платформы ИИ-сериалов.

## Архитектура

```
Фронтенд → Gateway (:8081) → Streaming Service (:8082)
                                  ↓
                            ffmpeg → H.264 + AAC
                                  ↓
                         MPEG-TS segments + M3U8 playlist
                                  ↓
                            HTTP Range / HLS
```

## Технологии

| Компонент | Назначение |
|-----------|-----------|
| **H.264 (libx264)** | Кодек сжатия видео |
| **AAC** | Кодек сжатия аудио |
| **MPEG-TS** | Контейнер для сегментов (.ts) |
| **M3U8** | HLS-плейлист |
| **ffmpeg** | Транскодирование исходного видео в HLS |
| **Go net/http** | Раздача плейлистов и сегментов |

## API

| Маршрут | Описание |
|---------|----------|
| `POST /upload` | Загрузить видео + создать эпизод в БД |
| `GET /stream/{videoID}/index.m3u8` | HLS-плейлист (M3U8) |
| `GET /stream/{videoID}/segment_001.ts` | MPEG-TS сегмент |
| `GET /videos/{videoID}` | Redirect → HLS-плейлист |
| `GET /health` | Health check |

### POST /upload

Загружает видеофайл, транскодирует в HLS и создаёт запись в таблице `episodes` с `tiktok_url` = URL стрима.

**multipart/form-data поля:**

| Поле | Тип | Обязательное | Описание |
|------|-----|-------------|----------|
| `file` | file | да | .mp4 видеофайл |
| `series_id` | int | да | ID сериала |
| `title` | string | нет | Название эпизода |
| `episode_num` | int | нет | Номер эпизода |

**Пример:**
```bash
curl -X POST http://localhost:8082/upload \
  -F "file=@episode1.mp4" \
  -F "series_id=1" \
  -F "title=Первая серия" \
  -F "episode_num=1"
```

**Ответ (201):**
```json
{"episode_id":1,"stream_url":"http://localhost:8082/stream/episode1/index.m3u8"}
```

## Запуск

### Требования

- Go 1.26+
- ffmpeg (должен быть в `PATH`)
- PostgreSQL (для `POST /upload` — создание эпизодов)

### Локально

```bash
# Положить .mp4 файлы в ./videos/
go run ./cmd/server/
```

Сервер запустится на `:8082`.

### Переменные окружения

| Переменная | По умолчанию | Описание |
|-----------|-------------|----------|
| `PORT` | `8082` | Порт сервера |
| `VIDEOS_DIR` | `./videos` | Директория с исходными .mp4 |
| `TRANSCODE_DIR` | `./transcoded` | Директория для HLS-вывода |
| `STREAM_BASE_URL` | `http://localhost:8082/stream` | Базовый URL для HLS-стримов (используется в `tiktok_url`) |
| `DATABASE_URL` | `postgres://admin:1@localhost:5432/series` | Строка подключения к PostgreSQL |

## Как это работает

1. Клиент запрашивает `/stream/{id}/index.m3u8`
2. Если HLS ещё не сгенерирован — сервер запускает ffmpeg:
   - Вход: `.mp4` файл
   - Выход: H.264 + AAC, упакованные в MPEG-TS сегменты по ~6 сек
   - Плейлист: M3U8 со списком всех сегментов
3. Клиент читает плейлист и запрашивает `.ts` сегменты по одному
4. `Cache-Control: max-age=86400` — сегменты кэшируются браузером/CDN
