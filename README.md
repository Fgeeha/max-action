# max-action

[![CI](https://github.com/Fgeeha/max-action/actions/workflows/ci.yml/badge.svg)](https://github.com/Fgeeha/max-action/actions/workflows/ci.yml)

🇬🇧 English | [🇷🇺 Русский](README.ru.md)

Send notifications to [MAX](https://max.ru) from GitHub Actions through the
official MAX Bot API. Use it to report pushes, pull requests, deployments and
failed builds to a chat, channel or direct message.

The action sends one text message per run and is written in Go with no
third-party dependencies.

## Quick start

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

## Examples

### Default message

Leave `message` out and the action builds a summary from the workflow context.
Lines whose values are unavailable are skipped.

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

### Notify only on failure

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

### Send a direct message to a user

```yaml
- uses: Fgeeha/max-action@v1
  with:
    token: ${{ secrets.MAX_TOKEN }}
    user_id: ${{ secrets.MAX_USER_ID }}
    message: Nightly job finished
```

### Read the message from a file

`message_file` takes precedence over `message`. Relative paths resolve against
`GITHUB_WORKSPACE`, so check out the repository first.

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

### Attach a file

`attachment_file` uploads a file (up to 50 MiB) and sends it with the message.
Relative paths resolve against `GITHUB_WORKSPACE`. The bot token goes only to
the MAX API; the upload host receives a pre-signed URL without it, and that URL
never appears in logs. The whole upload is capped at 5 minutes.

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

### Use the outputs

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

| Name | Required | Default | Description |
|------|----------|---------|-------------|
| `token` | yes | — | MAX bot access token. Always pass it from a secret. |
| `chat_id` | one of | — | Chat or channel ID. Mutually exclusive with `user_id`. |
| `user_id` | one of | — | User ID for a direct message. Mutually exclusive with `chat_id`. |
| `message` | no | — | Message text. Supports multiline values. |
| `message_file` | no | — | UTF-8 file whose contents override `message`. |
| `attachment_file` | no | — | File to send with the message, up to 50 MiB. Relative paths resolve against `GITHUB_WORKSPACE`. |
| `format` | no | `""` | `markdown`, `html`, or empty for plain text. |
| `notify` | no | `true` | Whether recipients get a notification. |
| `disable_link_preview` | no | `false` | Disable link previews. |
| `api_base_url` | no | `https://platform-api2.max.ru` | MAX API base URL. Mainly for testing against a mock server. |
| `timeout` | no | `15` | HTTP timeout per attempt, in seconds (`1`–`120`). |
| `retries` | no | `2` | Retry attempts after the first request (`0`–`5`). |
| `debug` | no | `false` | Enable safe diagnostic logging. |

Exactly one of `chat_id` and `user_id` must be set. Supplying both, or neither,
fails before any request is made.

The message text comes from the first available source: `message_file`, then
`message`, then the built-in template. It must be 1–4000 characters.

## Outputs

| Name | Description |
|------|-------------|
| `message_id` | The sent message ID, when the API returns one. May be empty. |
| `recipient_type` | `chat` or `user`. |
| `recipient_id` | The recipient ID the action used. |

A successful send never fails because `message_id` is missing: MAX is not
required to return it.

## Setting up secrets

1. Create a bot and obtain its access token through the MAX bot platform.
2. In the repository, open **Settings → Secrets and variables → Actions**.
3. Add `MAX_TOKEN` with the bot token.
4. Add `MAX_CHAT_ID` or `MAX_USER_ID` with the recipient ID.

Never put the token directly in a workflow file: everything in
`.github/workflows` is readable by anyone who can read the repository.

## `chat_id` versus `user_id`

| | `chat_id` | `user_id` |
|---|---|---|
| Target | Group chat or channel | One person, in a direct dialog |
| Typical use | Team notifications | Personal alerts |
| Value | May be negative | Positive user ID |

Consult the current [MAX developer documentation](https://dev.max.ru/docs-api)
for how to obtain these IDs: the recommended method has changed between API
versions, so this README deliberately does not restate it.

## MAX API limits

- A message is limited to 4000 characters. The action checks this before
  sending, so an oversized message fails without a wasted request.
- No more than two messages per second may be sent to one chat, channel or
  dialog. The action sends a single message per run, so it stays within the
  limit on its own; if you fan out several steps to the same chat, space them
  out yourself.
- A `429` response is retried according to the `Retry-After` header.
- The current API host is `platform-api2.max.ru`. The older
  `platform-api.max.ru` host is not used.

## Security

- The token is read from an input and immediately registered with
  `::add-mask::`, so the runner redacts it from all later log output.
- The token is sent only in the `Authorization` header. It never appears in a
  URL, query string, output or error message; API error text is scrubbed of the
  token before it reaches the log.
- TLS verification is always on and cannot be disabled.
- Redirects are refused rather than followed, so the `Authorization` header can
  never be replayed against a different host.
- `api_base_url` must be an absolute `http` or `https` URL, and plain HTTP is
  rejected outright for MAX hosts.
- Response bodies are capped at 1 MiB.
- API error text is stripped of control characters before it is written as a
  workflow command, so it cannot forge annotations.
- `debug` logging prints message length, recipient type, hostname, attempt
  number, HTTP status and duration. It never prints the token, request body or
  message text.
- `message_file` is only ever read, never executed. No input is passed to a
  shell.

Report vulnerabilities privately as described in [SECURITY.md](SECURITY.md).

### TLS and the MAX certificate chain

`platform-api2.max.ru` presents a certificate issued by the *Russian Trusted
Sub CA* of the Ministry of Digital Development. That root is not in the CA
stores shipped by GitHub-hosted runners or by standard container base images,
so a request would otherwise fail with `certificate verify failed`.

The action embeds the official bundle published at
`https://gu-st.ru/content/Other/doc/russiantrustedca.pem`, vendored in this
repository as `internal/maxapi/certs/russian_trusted_ca.pem`:

```text
sha256  6d1b66e7c1aa2512ad3abb50d6a6f144c9ee9d80fd7fbe1c9255a39f8e790944
```

Two properties are deliberate:

- The certificate is committed to the repository and compiled into the binary.
  Nothing is downloaded while the action runs, so the trust anchor is fixed at
  build time and reviewable in the diff.
- The bundle is added only to the certificate pool of this action's own HTTP
  client. The container's system trust store is left untouched, so the extra
  trust cannot affect anything else in the job.

## Troubleshooting

| Symptom | Cause and fix |
|---------|---------------|
| `exactly one of chat_id or user_id must be provided` | Both or neither were set. Provide exactly one. |
| `token must not be empty` | The secret is missing or misspelled. Secrets are not available to workflows triggered by a pull request from a fork. |
| `MAX API returned HTTP 401` | The token is wrong, revoked, or belongs to another bot. |
| `MAX API returned HTTP 403` | The bot is not a member of the chat, or lacks permission to post there. |
| `MAX API returned HTTP 404` | The `chat_id` or `user_id` does not exist. Check for a lost minus sign or a mistyped digit. |
| `message exceeds MAX limit of 4000 characters` | Shorten the text, or trim the file used by `message_file`. |
| `message is empty` | `message_file` pointed at an empty file. |
| `message_file not found` | The path resolves against `GITHUB_WORKSPACE` and needs `actions/checkout` to run first. |
| `attachment_file not found` / `exceeds 50 MiB` | Same path rules as `message_file`; files over 50 MiB are rejected before upload. |
| `attachment.not.ready` | MAX was still processing the file after 10 attempts 2 s apart. Retry the job. |
| `cannot get upload URL ...` | MAX refused `POST /uploads`: the same causes as HTTP 401/403 above, or a temporary MAX outage. |
| `cannot upload file: ...` | The upload host failed or timed out. The whole upload is capped at 5 minutes regardless of `timeout`. |
| `attachment_file is empty` / `is not a regular file` | Point `attachment_file` at a non-empty regular file, not a directory or device. |
| `redirects are not allowed` | `api_base_url` points at something that redirects. Use the direct API host. |
| Repeated `502`/`503` failures | MAX is unavailable. Raise `retries`, or `timeout` for slow networks. |

Set `debug: "true"` for per-attempt logging of status codes and timings. It
never reveals the token or the message text.

## Requirements and limitations

This is a Docker container action, so it runs only on **Linux runners**
(`ubuntu-latest` and other Linux images). It cannot run on Windows or macOS
runners.

The action pulls a prebuilt image from the GitHub Container Registry
(`ghcr.io/fgeeha/max-action`) rather than building the `Dockerfile` in your
workflow, so a step costs a few seconds instead of a full Go compile. The image
is built from this repository by the release workflow, is around 6 MB, and is
based on `scratch`: it contains the statically linked binary and a public CA
bundle, with no shell and no package manager.

The container runs as the default user so it can read the mounted workspace and
append to the runner-created `GITHUB_OUTPUT` file.

Text messages with at most one file attachment are supported. Images as
photos, video, audio and inline keyboards are not part of this release.

## Versioning

This project follows [Semantic Versioning](https://semver.org/). Releases are
tagged `vX.Y.Z`, and the moving `vX` tag always points at the newest release in
that major line.

Pin the major tag for automatic compatible updates:

```yaml
uses: Fgeeha/max-action@v1
```

Pin an exact release when you need reproducible builds:

```yaml
uses: Fgeeha/max-action@v1.1.1
```

Each release tag resolves to one immutable image tag, because `action.yml`
pins the exact `ghcr.io/fgeeha/max-action:vX.Y.Z` it was released with. The
release workflow refuses to publish when that pin does not match the tag being
released.

Do not use `@Master`: it tracks unreleased work, and its `action.yml` pins an
image tag that may not be published yet.

## License

[MIT](LICENSE)
