# Security Policy

## Supported versions

Security fixes are released for the latest `v1` release. Older tags are not
patched; move to the current `v1` release instead.

## Reporting a vulnerability

**Do not open a public issue for a security problem.**

Report it privately through GitHub's
[private vulnerability reporting](https://github.com/Fgeeha/max-action/security/advisories/new)
on this repository. If that is unavailable, contact the repository owner
directly through their GitHub profile.

Please include:

- what the issue is and why it is a security problem;
- the steps or workflow configuration needed to reproduce it;
- the action version or commit you tested;
- any impact you are aware of, such as token disclosure.

You can expect an initial response within 7 days. Once a fix is ready, it is
released and the advisory is published with credit, unless you prefer
otherwise.

## Scope

In scope:

- disclosure of the bot token in logs, outputs, errors or network requests;
- sending the `Authorization` header to an unintended host;
- weakening or bypassing TLS verification;
- injection through action inputs, including forged workflow commands;
- reading or executing files outside what `message_file` is meant to read.

Out of scope:

- vulnerabilities in the MAX API itself, which should go to MAX;
- workflows that deliberately print their own secrets;
- passing a token through an untrusted `api_base_url` on purpose.

## If a token is exposed

1. Revoke the bot token through the MAX bot platform immediately.
2. Issue a new token and update the repository secret.
3. Check the workflow run logs to see what was exposed and to whom.
