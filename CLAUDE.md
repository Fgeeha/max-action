# CLAUDE.md — Fgeeha/max-action

## 1. Назначение проекта

Разработать публичный GitHub Action `Fgeeha/max-action` для отправки уведомлений через официальный MAX Bot API.

По пользовательскому сценарию action должен быть похож на `appleboy/telegram-action`: разработчик подключает action в workflow, передаёт токен, получателя и текст, после чего уведомление отправляется при push, pull request, завершении deploy, ошибке CI и других событиях GitHub Actions.

Использовать `appleboy/telegram-action` только как ориентир по UX и структуре GitHub Action. Не копировать его исходный код, внутреннюю реализацию, тексты документации или Telegram-специфичные параметры.

## 2. Текущее состояние

- Репозиторий: `Fgeeha/max-action`.
- Репозиторий изначально пустой.
- Основная ветка на момент старта: `Master`.
- Проект создаётся с нуля.
- Не выполнять commit, push, release и публикацию в Marketplace без прямого запроса пользователя.

## 3. Главная цель MVP

MVP должен отправлять одно текстовое сообщение:

```text
POST https://platform-api2.max.ru/messages?chat_id=<id>
```

или:

```text
POST https://platform-api2.max.ru/messages?user_id=<id>
```

Токен передавать только HTTP-заголовком:

```text
Authorization: <access_token>
```

Тело запроса:

```json
{
  "text": "Deployment completed",
  "notify": true,
  "format": "markdown"
}
```

Параметр `disable_link_preview` передавать как query-параметр запроса, а не как поле JSON.

## 4. Обязательные ограничения MAX API

На момент реализации считать обязательными следующие правила:

1. Базовый URL по умолчанию — `https://platform-api2.max.ru`.
2. Старый домен `platform-api.max.ru` не использовать по умолчанию.
3. Токен нельзя передавать в query string, логах, сообщениях об ошибках или outputs.
4. Для отправки сообщения должен быть указан ровно один получатель:
   - `chat_id`; или
   - `user_id`.
5. Текст сообщения — от 1 до 4000 символов после чтения файла или формирования шаблона.
6. Допустимые значения `format`:
   - пустая строка;
   - `markdown`;
   - `html`.
7. `notify` по умолчанию равен `true`.
8. В один чат, канал или диалог нельзя отправлять более двух сообщений в секунду.
9. Не отключать проверку TLS ни при каких обстоятельствах.
10. Перед реализацией сетевого слоя проверить доверие GitHub-hosted runner и финального Docker image к сертификатной цепочке `platform-api2.max.ru`.
11. Если стандартное CA-хранилище не доверяет цепочке MAX, добавить только официальный сертификат Минцифры из первичного источника, закрепить checksum и описать происхождение сертификата в README. Не скачивать сертификат с неофициальных зеркал во время выполнения action.

API MAX может изменяться. Перед изменением контракта сверяться с официальной документацией MAX для разработчиков и changelog API.

## 5. Технологические решения

Реализовать action как Docker Action на Go.

Причины:

- один статически собранный бинарный файл;
- небольшое количество runtime-зависимостей;
- удобное тестирование HTTP-клиента через `httptest`;
- предсказуемая обработка exit codes;
- архитектура близка к контейнерной модели `appleboy/telegram-action`.

Требования:

- использовать актуальную стабильную версию Go;
- закрепить конкретную версию Go в `go.mod` и Dockerfile;
- по возможности использовать только стандартную библиотеку Go;
- Dockerfile должен быть multi-stage;
- финальный image должен содержать CA certificates и бинарный файл;
- запускать процесс не от root, если выбранный базовый image позволяет сделать это без неоправданного усложнения;
- поддерживать только Linux runners, что явно указать в README.

Не переписывать проект на JavaScript, Python или shell без явного решения пользователя.

## 6. Публичный интерфейс action

Создать `action.yml` со следующими inputs.

### Обязательные по смыслу

#### `token`

- Токен MAX-бота.
- В `action.yml` пометить как `required: true`.
- Сразу после чтения добавить маску GitHub Actions:

```text
::add-mask::<token>
```

#### `chat_id`

- ID группового чата или канала.
- Не является обязательным в `action.yml`, потому что вместо него может использоваться `user_id`.

#### `user_id`

- ID пользователя для личного сообщения.
- Не является обязательным в `action.yml`, потому что вместо него может использоваться `chat_id`.

В runtime валидировать, что заполнено ровно одно из полей `chat_id` и `user_id`.

### Текст сообщения

#### `message`

- Пользовательский текст сообщения.
- Поддерживает multiline input.
- Необязателен, потому что может использоваться `message_file` или стандартный шаблон.

#### `message_file`

- Путь к UTF-8 файлу внутри `GITHUB_WORKSPACE`.
- Если задан, содержимое файла имеет приоритет над `message`.
- Символ новой строки в конце файла не удалять без необходимости.
- Запретить чтение директории.
- Возвращать понятную ошибку, если файл отсутствует или недоступен.

Приоритет источников текста:

1. `message_file`;
2. `message`;
3. стандартное сообщение, сформированное из переменных GitHub Actions.

### Параметры MAX

#### `format`

- Допустимые значения: `markdown`, `html` или пустая строка.
- По умолчанию: пустая строка.

#### `notify`

- Включает обычное уведомление получателям.
- По умолчанию: `true`.

#### `disable_link_preview`

- Отключает генерацию preview ссылок.
- По умолчанию: `false`.

### Сетевые и диагностические параметры

#### `api_base_url`

- По умолчанию: `https://platform-api2.max.ru`.
- Нужен для тестирования через mock server и для будущих миграций API.
- Не разрешать URL без схемы.
- Для production-предупреждения не требуется, если используется значение по умолчанию.
- При нестандартном URL в debug-логе явно показывать hostname, но не токен.

#### `timeout`

- HTTP timeout в секундах.
- По умолчанию: `15`.
- Допустимый диапазон: `1..120`.

#### `retries`

- Максимальное число повторных попыток после первого запроса.
- По умолчанию: `2`.
- Допустимый диапазон: `0..5`.

#### `debug`

- По умолчанию: `false`.
- Не выводить полный request body, токен или потенциально секретное содержимое сообщения.
- Разрешено выводить длину сообщения, тип получателя, hostname, номер попытки, HTTP status и длительность запроса.

## 7. Outputs

Объявить следующие outputs:

### `message_id`

- Значение `message.body.mid`, если оно присутствует в успешном ответе MAX.
- Отсутствие `mid` не должно превращать успешный HTTP-ответ в ошибку.

### `recipient_type`

- `chat` или `user`.

### `recipient_id`

- Фактически использованный `chat_id` или `user_id`.

Не публиковать полный API response как output: он может содержать лишние данные.

Записывать outputs только через файл из переменной `GITHUB_OUTPUT`.

## 8. Стандартное сообщение

Если не заданы `message_file` и `message`, сформировать компактное сообщение:

```text
<actor> triggered <workflow>

Repository: <repository>
Event: <event_name>
Ref: <ref>
Commit: <sha>
Run: <server_url>/<repository>/actions/runs/<run_id>
```

Правила:

- не падать, если отдельная GitHub-переменная отсутствует;
- не читать произвольные поля event JSON для MVP;
- не включать environment variables целиком;
- не включать secrets;
- SHA можно сокращать до 7 символов только в отображаемом тексте;
- URL запуска строить только при наличии всех необходимых частей.

## 9. Валидация конфигурации

До любого HTTP-запроса проверить:

- `token` не пустой;
- задан ровно один target;
- ID является целым числом в диапазоне signed int64;
- текст не пустой;
- текст не превышает 4000 символов;
- `format` допустим;
- boolean inputs распознаны строго;
- `timeout` и `retries` находятся в допустимых диапазонах;
- `api_base_url` является корректным absolute URL с `http` или `https`;
- для стандартного MAX endpoint разрешён только HTTPS.

Ошибки конфигурации должны быть короткими и понятными. Для пользовательских ошибок не печатать stack trace.

Примеры:

```text
exactly one of chat_id or user_id must be provided
message exceeds MAX limit of 4000 characters
format must be one of: markdown, html, or empty
```

## 10. HTTP-клиент

Создать отдельный пакет клиента MAX API.

Обязательное поведение:

1. Формировать URL через `net/url`, без конкатенации пользовательских значений.
2. Использовать `POST /messages`.
3. Передавать target и `disable_link_preview` через query parameters.
4. Передавать `Authorization` и `Content-Type: application/json` через headers.
5. Добавить User-Agent, например:

```text
Fgeeha/max-action/<version>
```

6. Принимать любой HTTP status `2xx` как успешный.
7. Ограничить размер читаемого response body, например 1 MiB.
8. Закрывать response body во всех ветках.
9. Использовать context timeout.
10. Не повторять запрос после `400`, `401`, `403` или `404`.
11. Повторять запрос только при:
    - network error, который имеет смысл повторить;
    - `429`;
    - `502`;
    - `503`;
    - `504`.
12. Для `429` учитывать `Retry-After`, если заголовок корректен.
13. Для остальных retry использовать exponential backoff с небольшим jitter.
14. Не превышать общий context timeout бесконтрольными sleep.
15. При окончательной ошибке возвращать status code и безопасно сокращённое сообщение API.
16. Перед выводом ошибок проверять, что токен не оказался в URL, body или тексте ошибки.

Action отправляет только одно сообщение за запуск, поэтому отдельная очередь для лимита два сообщения в секунду в MVP не нужна. Retry после `429` должен соблюдать ответ сервера.

## 11. Работа с ответом MAX

Ожидаемый успешный ответ содержит объект сообщения.

Декодер должен:

- корректно обрабатывать documented response;
- извлекать `body.mid`, когда поле присутствует;
- не требовать наличия необязательных полей;
- не падать на дополнительных неизвестных JSON-полях;
- считать пустой response body допустимым для любого успешного `2xx`, но выводить debug warning;
- выдавать понятную ошибку при невалидном JSON только тогда, когда response body непустой и результат необходим для обработки.

Не связывать успешность отправки исключительно с наличием `message_id`.

## 12. Предлагаемая структура проекта

```text
.
├── .github/
│   ├── workflows/
│   │   ├── ci.yml
│   │   └── release.yml
│   └── dependabot.yml
├── cmd/
│   └── max-action/
│       └── main.go
├── internal/
│   ├── action/
│   │   ├── runner.go
│   │   └── runner_test.go
│   ├── config/
│   │   ├── config.go
│   │   └── config_test.go
│   ├── github/
│   │   ├── output.go
│   │   └── output_test.go
│   ├── maxapi/
│   │   ├── client.go
│   │   ├── client_test.go
│   │   └── types.go
│   └── message/
│       ├── resolver.go
│       └── resolver_test.go
├── action.yml
├── Dockerfile
├── .dockerignore
├── .gitignore
├── go.mod
├── go.sum
├── LICENSE
├── README.md
├── CHANGELOG.md
├── SECURITY.md
└── CLAUDE.md
```

Структуру можно упростить, если отдельный пакет содержит слишком мало логики. Не создавать абстракции без реальной необходимости.

## 13. Dockerfile

Использовать multi-stage build:

1. Builder stage:
   - закреплённый официальный Go image;
   - `CGO_ENABLED=0`;
   - сборка с `-trimpath`;
   - по возможности удалить лишнюю build-информацию через linker flags;
   - кешировать модули корректным порядком `COPY`.
2. Runtime stage:
   - минимальный поддерживаемый image;
   - CA certificates;
   - официальный сертификат Минцифры только при подтверждённой необходимости;
   - бинарный файл;
   - `WORKDIR /github/workspace`;
   - `ENTRYPOINT` на бинарный файл.

Не использовать `latest` в базовых images.

Не скачивать зависимости или сертификаты при каждом запуске action.

## 14. `action.yml`

Минимальная форма:

```yaml
name: MAX Message Notify
description: Send notifications to MAX from GitHub Actions
author: Fgeeha

inputs:
  token:
    description: MAX bot access token
    required: true
  chat_id:
    description: MAX chat or channel ID
    required: false
  user_id:
    description: MAX user ID
    required: false
  message:
    description: Message text
    required: false
  message_file:
    description: UTF-8 file whose contents override message
    required: false
  format:
    description: Message format: markdown, html, or empty
    required: false
    default: ""
  notify:
    description: Notify recipients
    required: false
    default: "true"
  disable_link_preview:
    description: Disable link previews
    required: false
    default: "false"
  api_base_url:
    description: MAX API base URL
    required: false
    default: https://platform-api2.max.ru
  timeout:
    description: HTTP timeout in seconds
    required: false
    default: "15"
  retries:
    description: Number of retry attempts
    required: false
    default: "2"
  debug:
    description: Enable safe debug logging
    required: false
    default: "false"

outputs:
  message_id:
    description: Sent MAX message ID when returned by API
  recipient_type:
    description: Recipient type: chat or user
  recipient_id:
    description: Recipient ID used by the action

runs:
  using: docker
  image: Dockerfile

branding:
  icon: message-square
  color: blue
```

Перед завершением проверить `action.yml` по актуальному синтаксису GitHub Actions metadata.

## 15. Логи GitHub Actions

Использовать workflow commands там, где это улучшает UX:

```text
::add-mask::<token>
::error::<message>
::warning::<message>
::notice::<message>
```

Обычный успешный вывод должен быть компактным:

```text
MAX message sent successfully to chat <id>
```

Не выводить:

- token;
- полный Authorization header;
- полный API response;
- все environment variables;
- message body в debug-режиме;
- содержимое файлов, если оно может быть секретным.

## 16. Тестирование

Все ключевые сценарии должны иметь автоматические unit tests.

### Config tests

- только `chat_id`;
- только `user_id`;
- оба target одновременно;
- target отсутствует;
- пустой token;
- отрицательный chat ID допускается, если он валиден для API;
- нечисловой ID;
- invalid bool;
- invalid format;
- timeout вне диапазона;
- retries вне диапазона;
- invalid base URL;
- HTTP для production endpoint запрещён.

### Message resolver tests

- `message_file` имеет приоритет;
- multiline message сохраняется;
- отсутствующий файл;
- путь указывает на директорию;
- пустой файл;
- UTF-8 текст;
- сообщение длиной 4000 символов;
- сообщение длиной 4001 символ;
- стандартный шаблон при отсутствии пользовательского текста.

### MAX client tests через `httptest.Server`

Проверить:

- HTTP method и path;
- query с `chat_id`;
- query с `user_id`;
- query с `disable_link_preview`;
- Authorization header;
- JSON body;
- отсутствие пустого `format` в JSON либо согласованное поведение;
- успешный `200`;
- успешный другой `2xx`;
- извлечение `body.mid`;
- успешный ответ без `mid`;
- `400` без retry;
- `401` без retry;
- `429` с `Retry-After`;
- `503` с retry;
- network timeout;
- malformed JSON;
- слишком большой response body;
- токен отсутствует в возвращаемой ошибке.

Запускать тесты с race detector в CI:

```bash
go test -race ./...
```

## 17. CI

Создать `.github/workflows/ci.yml` со следующими проверками:

1. checkout;
2. setup-go с версией из `go.mod`;
3. проверка форматирования;
4. `go vet ./...`;
5. `go test -race -coverprofile=coverage.out ./...`;
6. `go build ./cmd/max-action`;
7. сборка Docker image;
8. проверка, что рабочее дерево не изменилось после генерации или тестов.

Минимальная проверка форматирования:

```bash
test -z "$(gofmt -l .)"
```

Не отправлять настоящее сообщение MAX в обычном CI.

Для live integration test создать отдельный workflow с `workflow_dispatch`, environment protection и secrets:

- `MAX_TOKEN`;
- `MAX_CHAT_ID` или `MAX_USER_ID`.

Live test не должен запускаться на pull request из fork.

## 18. README

README должен содержать:

1. краткое описание;
2. статус CI;
3. минимальный пример использования;
4. пример для push;
5. пример уведомления только при failure;
6. пример `message_file`;
7. таблицу inputs;
8. таблицу outputs;
9. настройку secrets;
10. объяснение различий `chat_id` и `user_id`;
11. способ получения `chat_id` только по актуальной документации MAX;
12. ограничения MAX API;
13. раздел Security;
14. раздел Troubleshooting;
15. ограничения Docker Action и Linux runners;
16. политику версионирования.

Основной пример должен использовать стабильный major tag, а не ветку:

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

Пример уведомления при ошибке:

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

## 19. Безопасность

Обязательные правила:

- token хранится только в GitHub Secrets;
- token маскируется до любых потенциальных логов;
- token никогда не передаётся в URL;
- TLS verification всегда включена;
- `api_base_url` не должен позволять случайно вывести token в redirect на другой host;
- по умолчанию не следовать cross-host redirects с Authorization header;
- лучше полностью запретить redirects для POST-запроса, если API их не требует;
- response body ограничен по размеру;
- ошибки API очищаются от управляющих символов перед workflow command;
- переносы строк в `::error::` безопасно нормализуются;
- не выполнять shell-команды на основе input values;
- `message_file` только читается и никогда не исполняется;
- не использовать `InsecureSkipVerify`;
- не логировать GitHub event payload целиком.

Добавить `SECURITY.md` с инструкцией сообщать об уязвимостях приватно, а не через публичный issue.

## 20. Обработка redirects

HTTP-клиент не должен автоматически пересылать Authorization header на другой hostname.

Предпочтительное поведение:

- запретить redirects и вернуть понятную ошибку;
- либо разрешить redirect только на тот же scheme и hostname;
- для стандартного MAX endpoint HTTPS нельзя понижать до HTTP.

Покрыть это тестами.

## 21. Версионирование и релизы

Использовать Semantic Versioning.

Рекомендуемый процесс:

1. создать release tag, например `v1.0.0`;
2. обновить moving major tag `v1` на тот же commit;
3. создать GitHub Release с changelog;
4. только после проверки README и metadata публиковать action в Marketplace.

Никогда автоматически не переписывать major tag без release workflow и явного решения владельца.

`release.yml` должен иметь минимально необходимые permissions.

## 22. Roadmap после MVP

Не включать эти функции в первый PR, пока текстовый MVP не завершён:

- изображения;
- документы;
- видео;
- аудио;
- inline keyboard;
- отправка нескольким получателям;
- шаблонизатор;
- автоматическое разбиение сообщения длиннее 4000 символов;
- proxy support;
- редактирование и удаление сообщений.

После MVP следующий разумный этап:

1. `attachments` в виде готового JSON;
2. загрузка image/file через `POST /uploads`;
3. inline keyboard;
4. несколько recipients с соблюдением rate limit.

Каждое расширение должно появляться отдельным PR и иметь тесты.

## 23. Не делать

- Не копировать набор Telegram inputs один к одному.
- Не добавлять Telegram terminology в публичный API.
- Не использовать устаревший MAX API hostname по умолчанию.
- Не передавать token query-параметром.
- Не отключать TLS verification.
- Не использовать shell `eval`.
- Не добавлять тяжёлый framework ради одного HTTP-запроса.
- Не создавать собственный SDK MAX внутри action.
- Не добавлять скрытые telemetry или analytics.
- Не выполнять live API requests из unit tests.
- Не публиковать release без ручной проверки.
- Не использовать `@master` в рекомендуемых пользовательских примерах.

## 24. Порядок выполнения работы Claude Code

При начале новой задачи:

1. прочитать `CLAUDE.md` полностью;
2. проверить текущее состояние репозитория и `git status`;
3. изучить существующие файлы до внесения изменений;
4. сверить затрагиваемый контракт с актуальной официальной документацией MAX;
5. сформировать короткий план;
6. реализовать минимально достаточное изменение;
7. добавить или обновить тесты;
8. выполнить форматирование, unit tests, vet и build;
9. проверить diff на утечки token, временные файлы и лишние изменения;
10. дать итоговый отчёт.

Не останавливаться после написания кода без запуска доступных проверок.

## 25. Definition of Done для MVP

MVP считается готовым, когда:

- существует корректный `action.yml`;
- Docker image собирается;
- action принимает `token` и ровно один target;
- action отправляет текст через `POST /messages`;
- используется `platform-api2.max.ru`;
- Authorization передаётся заголовком;
- поддерживаются `message`, `message_file`, `format`, `notify` и `disable_link_preview`;
- token отсутствует в логах;
- ошибки API приводят к ненулевому exit code;
- успешная отправка приводит к exit code 0;
- outputs записываются через `GITHUB_OUTPUT`;
- unit tests покрывают config, resolver и HTTP client;
- `go test -race ./...`, `go vet ./...`, build и Docker build проходят;
- README содержит готовые примеры;
- live integration test отделён от обычного CI;
- нет TODO, блокирующих базовую отправку текста.

## 26. Формат итогового отчёта Claude

После завершения задачи сообщить:

```text
Implemented
- ...

Validation
- gofmt: passed
- go vet ./...: passed
- go test -race ./...: passed
- go build ./cmd/max-action: passed
- docker build: passed

Important decisions
- ...

Remaining work
- ...
```

Не заявлять, что проверка прошла, если команда фактически не запускалась.
