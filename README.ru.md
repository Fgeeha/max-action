# max-action

[![CI](https://github.com/Fgeeha/max-action/actions/workflows/ci.yml/badge.svg)](https://github.com/Fgeeha/max-action/actions/workflows/ci.yml)

[🇬🇧 English](README.md) | 🇷🇺 Русский

Отправка уведомлений в [MAX](https://max.ru) из GitHub Actions через
официальный MAX Bot API. Используется для сообщений о push, pull request,
деплое и упавших сборках в чат, канал или личные сообщения.

Action отправляет одно текстовое сообщение за запуск и написан на Go без
сторонних зависимостей.

## Быстрый старт

```yaml
name: MAX notification

on:
  push:

jobs:
  notify:
    runs-on: ubuntu-latest
    steps:
      - name: Send MAX message
        uses: Fgeeha/max-action@v1
        with:
          token: ${{ secrets.MAX_TOKEN }}
          chat_id: ${{ secrets.MAX_CHAT_ID }}
          message: |
            ${{ github.actor }} pushed a commit

            Repository: ${{ github.repository }}
            Commit: ${{ github.sha }}
            Run: ${{ github.server_url }}/${{ github.repository }}/actions/runs/${{ github.run_id }}
```

## Примеры

### Сообщение по умолчанию

Если не указать `message`, action соберёт сводку из контекста workflow.
Строки, для которых значения недоступны, пропускаются.

```yaml
- uses: Fgeeha/max-action@v1
  with:
    token: ${{ secrets.MAX_TOKEN }}
    chat_id: ${{ secrets.MAX_CHAT_ID }}
```

```text
octocat triggered CI

Repository: Fgeeha/max-action
Event: push
Ref: refs/heads/Master
Commit: 0123456
Run: https://github.com/Fgeeha/max-action/actions/runs/42
```

### Уведомление только при ошибке

```yaml
- name: Send failure notification
  if: ${{ failure() }}
  uses: Fgeeha/max-action@v1
  with:
    token: ${{ secrets.MAX_TOKEN }}
    chat_id: ${{ secrets.MAX_CHAT_ID }}
    format: markdown
    message: |
      ❌ Workflow failed

      Repository: ${{ github.repository }}
      Workflow: ${{ github.workflow }}
      Run: ${{ github.server_url }}/${{ github.repository }}/actions/runs/${{ github.run_id }}
```

### Личное сообщение пользователю

```yaml
- uses: Fgeeha/max-action@v1
  with:
    token: ${{ secrets.MAX_TOKEN }}
    user_id: ${{ secrets.MAX_USER_ID }}
    message: Nightly job finished
```

### Чтение сообщения из файла

`message_file` имеет приоритет над `message`. Относительные пути
разрешаются относительно `GITHUB_WORKSPACE`, поэтому сначала выполните
checkout репозитория.

```yaml
- uses: actions/checkout@v4

- name: Build release notes
  run: git log -1 --pretty=%B > notes.txt

- uses: Fgeeha/max-action@v1
  with:
    token: ${{ secrets.MAX_TOKEN }}
    chat_id: ${{ secrets.MAX_CHAT_ID }}
    message_file: notes.txt
```

### Вложение файла

`attachment_file` загружает файл (до 50 МиБ) и отправляет его вместе с
сообщением. Относительные пути разрешаются относительно `GITHUB_WORKSPACE`.
Токен бота уходит только в MAX API: хост загрузки получает подписанный URL
без него, и этот URL не попадает в логи. Вся загрузка ограничена 5 минутами.

```yaml
- name: Pack the docs
  run: zip -qr docs.zip docs/html

- uses: Fgeeha/max-action@v1
  with:
    token: ${{ secrets.MAX_TOKEN }}
    chat_id: ${{ secrets.MAX_CHAT_ID }}
    message: Documentation for ${{ github.sha }}
    attachment_file: docs.zip
```

### Использование outputs

```yaml
- id: notify
  uses: Fgeeha/max-action@v1
  with:
    token: ${{ secrets.MAX_TOKEN }}
    chat_id: ${{ secrets.MAX_CHAT_ID }}
    message: Deployment started

- run: echo "Sent message ${{ steps.notify.outputs.message_id }}"
```

## Inputs

| Имя | Обязателен | По умолчанию | Описание |
|------|----------|---------|-------------|
| `token` | да | — | Токен доступа MAX-бота. Передавайте только из secret. |
| `chat_id` | один из двух | — | ID чата или канала. Взаимоисключающий с `user_id`. |
| `user_id` | один из двух | — | ID пользователя для личного сообщения. Взаимоисключающий с `chat_id`. |
| `message` | нет | — | Текст сообщения. Поддерживает multiline. |
| `message_file` | нет | — | UTF-8 файл, содержимое которого переопределяет `message`. |
| `attachment_file` | нет | — | Файл для отправки вместе с сообщением, до 50 МиБ. Относительные пути — от `GITHUB_WORKSPACE`. |
| `format` | нет | `""` | `markdown`, `html` или пусто для обычного текста. |
| `notify` | нет | `true` | Получат ли получатели уведомление. |
| `disable_link_preview` | нет | `false` | Отключить preview ссылок. |
| `api_base_url` | нет | `https://platform-api2.max.ru` | Базовый URL MAX API. В основном для тестов против mock-сервера. |
| `timeout` | нет | `15` | HTTP timeout на попытку, в секундах (`1`–`120`). |
| `retries` | нет | `2` | Число повторов после первого запроса (`0`–`5`). |
| `debug` | нет | `false` | Включить безопасное диагностическое логирование. |

Должно быть задано ровно одно из `chat_id` и `user_id`. Если заданы оба или
ни одного — action завершится ошибкой до отправки запроса.

Текст сообщения берётся из первого доступного источника: `message_file`,
затем `message`, затем встроенный шаблон. Длина — 1–4000 символов.
`message_file` должен быть обычным файлом размером не более 16 000 байт
(4000 символов по 4 байта максимум); файлы большего размера отклоняются без
чтения целиком.

## Outputs

| Имя | Описание |
|------|-------------|
| `message_id` | ID отправленного сообщения, если API его вернул. Может быть пустым. |
| `recipient_type` | `chat` или `user`. |
| `recipient_id` | Фактически использованный ID получателя. |

Успешная отправка никогда не считается ошибкой из-за отсутствия
`message_id`: MAX не обязан его возвращать.

## Настройка secrets

1. Создайте бота и получите его токен доступа через MAX bot platform.
2. В репозитории откройте **Settings → Secrets and variables → Actions**.
3. Добавьте `MAX_TOKEN` с токеном бота.
4. Добавьте `MAX_CHAT_ID` или `MAX_USER_ID` с ID получателя.

Никогда не указывайте токен прямо в workflow-файле: всё в
`.github/workflows` доступно на чтение всем, кто может читать репозиторий.

## `chat_id` против `user_id`

| | `chat_id` | `user_id` |
|---|---|---|
| Цель | Групповой чат или канал | Один человек, личный диалог |
| Типичное применение | Командные уведомления | Личные оповещения |
| Значение | Может быть отрицательным | Положительный ID пользователя |

Актуальный способ получения этих ID смотрите в текущей
[документации разработчика MAX](https://dev.max.ru/docs-api): он менялся
между версиями API, поэтому этот README сознательно его не переписывает.

## Ограничения MAX API

- Сообщение ограничено 4000 символами. Action проверяет это перед
  отправкой, поэтому слишком длинное сообщение не тратит лишний запрос.
- В один чат, канал или диалог нельзя отправлять более двух сообщений в
  секунду. Action отправляет одно сообщение за запуск, поэтому сам по себе
  укладывается в лимит; если несколько шагов пишут в один и тот же чат,
  разносите их по времени самостоятельно.
- Ответ `429` повторяется с учётом заголовка `Retry-After`.
- `POST /messages` не идемпотентен. Если MAX принял сообщение, но ответ
  потерялся по дороге (обрыв соединения или таймаут), action не может
  отличить это от недошедшего запроса и повторяет его, поэтому получатель
  может увидеть сообщение дважды. `retries: "0"` уменьшает этот риск ценой
  отказа при любом временном сбое и всё равно не гарантирует однократную
  доставку. Ответ `2xx`, тело которого не удалось прочитать, считается
  успешной отправкой: шаг завершается успешно с пустым `message_id`, повтора
  нет.
- Текущий хост API — `platform-api2.max.ru`. Старый хост
  `platform-api.max.ru` не используется.

## Безопасность

- Токен читается из input и сразу регистрируется через `::add-mask::`,
  поэтому runner скрывает его во всех последующих логах.
- Токен передаётся только в заголовке `Authorization`. Он никогда не
  попадает в URL, query string, output или сообщение об ошибке; текст
  ошибки API очищается от токена перед выводом в лог.
- Проверка TLS всегда включена и не может быть отключена.
- Редиректы отклоняются, а не пересылаются, поэтому заголовок
  `Authorization` никогда не может быть повторно отправлен на другой хост.
- `api_base_url` должен быть абсолютным `http`- или `https`-URL, а обычный
  HTTP отклоняется для хостов MAX.
- Тело ответа ограничено 1 MiB.
- Текст ошибки API очищается от управляющих символов перед записью в виде
  workflow command, поэтому не может подделать аннотации.
- `debug`-логирование выводит длину сообщения, тип получателя, hostname,
  номер попытки, HTTP-статус и длительность. Токен, тело запроса и текст
  сообщения никогда не выводятся.
- `message_file` только читается и никогда не исполняется. Ни один input не
  передаётся в shell.

Сообщать об уязвимостях следует приватно, как описано в
[SECURITY.md](SECURITY.md).

### TLS и цепочка сертификатов MAX

`platform-api2.max.ru` предъявляет сертификат, выданный *Russian Trusted
Sub CA* Министерства цифрового развития. Этот корневой сертификат
отсутствует в CA-хранилищах, поставляемых GitHub-hosted runner'ами и
стандартными базовыми образами контейнеров, поэтому без него запрос
завершался бы ошибкой `certificate verify failed`.

Action включает в себя официальный бандл, опубликованный по адресу
`https://gu-st.ru/content/Other/doc/russiantrustedca.pem`, добавленный в
репозиторий как `internal/maxapi/certs/russian_trusted_ca.pem`:

```text
sha256  6d1b66e7c1aa2512ad3abb50d6a6f144c9ee9d80fd7fbe1c9255a39f8e790944
```

Два свойства сделаны намеренно:

- Сертификат закоммичен в репозиторий и вкомпилирован в бинарный файл.
  Во время выполнения action ничего не скачивается, поэтому trust anchor
  зафиксирован на этапе сборки и виден в diff'е.
- Бандл добавляется только в пул сертификатов собственного HTTP-клиента
  action. Системное хранилище доверия контейнера не затрагивается, поэтому
  дополнительное доверие не может повлиять ни на что другое в job.

Бандл нужно обновить до истечения срока: сертификат *Russian Trusted Sub CA*
действителен до 6 марта 2027 года. `TestEmbeddedCANotExpiring` начинает
падать за 90 дней до этой даты, и CI запускает его еженедельно по
расписанию. Однако GitHub отключает расписание после 60 дней без активности
в репозитории, поэтому в неактивном репозитории эта проверка сама
остановится. Заведите календарное напоминание на дату обновления или
запускайте CI-workflow из внешнего планировщика через `workflow_dispatch`.

## Troubleshooting

| Симптом | Причина и решение |
|---------|---------------|
| `exactly one of chat_id or user_id must be provided` | Заданы оба или ни одного. Укажите ровно один. |
| `token must not be empty` | Secret отсутствует или указан с опечаткой. Secrets недоступны для workflow, запущенных pull request'ом из форка. |
| `MAX API returned HTTP 401` | Токен неверный, отозван или принадлежит другому боту. |
| `MAX API returned HTTP 403` | Бот не состоит в чате или не имеет прав на отправку сообщений в него. |
| `MAX API returned HTTP 404` | `chat_id` или `user_id` не существует. Проверьте потерянный минус или опечатку в цифре. |
| `message exceeds MAX limit of 4000 characters` | Сократите текст или уменьшите файл, используемый в `message_file`. |
| `message is empty` | `message_file` указывает на пустой файл. |
| `message_file not found` | Путь разрешается относительно `GITHUB_WORKSPACE`, для этого сначала нужно выполнить `actions/checkout`. |
| `message_file exceeds 16000 bytes` / `is not a regular file` | Укажите в `message_file` обычный файл не больше 16 000 байт, а не канал или устройство. |
| `attachment_file not found` / `exceeds 50 MiB` | Правила путей те же, что у `message_file`; файлы больше 50 МиБ отклоняются до загрузки. |
| `attachment.not.ready` | MAX не успел обработать файл за 10 попыток с паузой 2 с. Перезапустите job. |
| `cannot get upload URL ...` | MAX отклонил `POST /uploads`: те же причины, что у HTTP 401/403 выше, или временный сбой MAX. |
| `cannot upload file: ...` | Хост загрузки ответил ошибкой или не уложился во время. Вся загрузка ограничена 5 минутами независимо от `timeout`. |
| `attachment_file is empty` / `is not a regular file` | Укажите в `attachment_file` непустой обычный файл, а не каталог или устройство. |
| `redirects are not allowed` | `api_base_url` указывает на что-то, что делает редирект. Используйте прямой хост API. |
| Повторяющиеся ошибки `502`/`503` | MAX недоступен. Увеличьте `retries` или `timeout` для медленных сетей. |
| Одно и то же сообщение пришло дважды | MAX принял первую попытку, но ответ потерялся, и action повторил запрос. См. [Ограничения MAX API](#ограничения-max-api). |

Установите `debug: "true"` для логирования каждой попытки со статус-кодами
и таймингами. Токен и текст сообщения при этом никогда не раскрываются.

## Требования и ограничения

Это Docker container action, поэтому он работает только на **Linux
runner'ах** (`ubuntu-latest` и другие Linux-образы). Он не может работать
на Windows- или macOS-runner'ах.

Action скачивает предсобранный образ из GitHub Container Registry
(`ghcr.io/fgeeha/max-action`) вместо сборки `Dockerfile` в вашем workflow,
поэтому шаг занимает несколько секунд вместо полной компиляции Go. Образ
собирается из этого репозитория release-workflow'ом, весит около 6 МБ и
основан на `scratch`: содержит статически слинкованный бинарный файл и
публичный CA-бандл, без shell и без пакетного менеджера.

Контейнер запускается от пользователя по умолчанию, чтобы иметь доступ на
чтение к смонтированному workspace и дописывать в созданный runner'ом файл
`GITHUB_OUTPUT`.

Поддерживаются текстовые сообщения и не больше одного вложения-файла.
Изображения как фото, видео, аудио и inline-клавиатуры в этот релиз не
входят.

## Версионирование

Проект следует [Semantic Versioning](https://semver.org/). Релизы
помечаются тегами `vX.Y.Z`, а плавающий тег `vX` всегда указывает на
последний релиз этой major-версии.

Закрепите major-тег для автоматических совместимых обновлений:

```yaml
uses: Fgeeha/max-action@v1
```

Закрепите точный релиз, если нужна воспроизводимая сборка:

```yaml
uses: Fgeeha/max-action@v1.1.2
```

Каждый релизный тег разрешается ровно в один immutable тег образа, потому
что `action.yml` фиксирует точный `ghcr.io/fgeeha/max-action:vX.Y.Z`, с
которым был выпущен релиз. Release-workflow отказывается публиковать
релиз, если этот пин не совпадает с публикуемым тегом.

Не используйте `@Master`: эта ветка отслеживает неопубликованные
изменения, и её `action.yml` может фиксировать ещё не опубликованный тег
образа.

## Лицензия

[MIT](LICENSE)
