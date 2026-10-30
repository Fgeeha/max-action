# Changelog

All notable changes to this project are documented in this file.

The format is based on [Keep a Changelog](https://keepachangelog.com/en/1.1.0/),
and this project adheres to [Semantic Versioning](https://semver.org/).

## [Unreleased]

### Fixed

- `api_base_url` с завершающей точкой в имени хоста (`http://platform-api2.max.ru.`)
  больше не обходит запрет HTTP для хостов MAX.
- Ответ `2xx`, тело которого оборвалось или превысило 1 MiB, больше не
  считается ошибкой: шаг завершается успешно с пустым `message_id` и
  предупреждением, повторной отправки нет.
- `message_file` читается не более чем на 16 000 байт; файлы большего размера,
  каналы и устройства отклоняются.
- Release workflow перемещает major-тег только для старшей версии в этой
  major-линии: повторный запуск старого релиза больше не откатывает `v1`.

### Documentation

- Описан риск дублирования сообщения при повторе после потерянного ответа.
- Описан срок действия встроенного сертификата и ограничение расписания CI в
  неактивном репозитории.

## [1.1.1] - 2026-10-30

### Fixed

- A network error during upload no longer prints the pre-signed upload URL
  in the error or debug log; only its host is shown.
- The upload is capped at 5 minutes in total instead of 5 minutes per
  attempt.
- The file is held in memory once instead of twice during upload.
- MAX error codes appear in the error text, e.g.
  `MAX API returned HTTP 400 (attachment.not.ready): ...`, so they match the
  troubleshooting table.
- The redirect error no longer names only the MAX API, since the upload host
  is refused the same way.

### Changed

- The wait for attachment processing grows from 5 to 10 attempts, 2 seconds
  apart.
- The debug log no longer names the attached file.

## [1.1.0] - 2026-10-30

### Added

- `attachment_file` input: uploads a file of up to 50 MiB through
  `POST /uploads?type=file` and sends it with the message. The bot token is
  sent only to the MAX API; the upload host receives a pre-signed URL
  without `Authorization`.
- While MAX reports `attachment.not.ready`, the send is retried up to 5 times
  2 seconds apart.
- The live integration test attaches a small archive.

## [1.0.1] - 2026-08-16

### Changed

- The action now pulls a prebuilt image from
  `ghcr.io/fgeeha/max-action` instead of building the `Dockerfile` in every
  consumer workflow, which removes a Go compile from each run.
- The runtime image is based on `scratch` instead of Alpine: about 6 MB
  instead of 14 MB, with no shell and no package manager.
- Go 1.26.4 and Alpine 3.24 for the build stage.

### Fixed

- The version compiled into the binary, and therefore the `User-Agent`, is now
  the released tag. It was always `dev`, because GitHub cannot pass build
  arguments to an inline `image: Dockerfile` action.

### Added

- A test that fails 90 days before any vendored MAX certificate expires, and a
  weekly CI schedule so a dormant repository still reports it.
- The release workflow verifies that `action.yml` pins the image tag being
  released before anything is published.

### Security

- Third-party actions in every workflow are pinned by commit SHA.

## [1.0.0] - 2026-07-30

### Added

- Send a single text message to a MAX chat, channel or user through
  `POST /messages` on `https://platform-api2.max.ru`.
- Inputs `token`, `chat_id`, `user_id`, `message`, `message_file`, `format`,
  `notify`, `disable_link_preview`, `api_base_url`, `timeout`, `retries` and
  `debug`.
- Outputs `message_id`, `recipient_type` and `recipient_id`, written through
  `GITHUB_OUTPUT`.
- A default message built from the GitHub Actions context when no text is
  supplied.
- Retries with exponential backoff and jitter for `429`, `502`, `503`, `504`
  and transient network failures, honouring `Retry-After`.
- The official Russian Trusted CA bundle embedded in the binary, so the MAX
  certificate chain verifies without touching the system trust store.

### Security

- The token is masked with `::add-mask::` before anything else runs, is sent
  only in the `Authorization` header, and is redacted from error text.
- Redirects are refused so the `Authorization` header cannot be replayed
  against another host.
- Plain HTTP is rejected for MAX hosts, response bodies are capped at 1 MiB,
  and API error text is stripped of control characters before being written as
  a workflow command.
