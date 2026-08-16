# Changelog

All notable changes to this project are documented in this file.

The format is based on [Keep a Changelog](https://keepachangelog.com/en/1.1.0/),
and this project adheres to [Semantic Versioning](https://semver.org/).

## [Unreleased]

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
