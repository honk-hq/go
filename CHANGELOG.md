# Changelog

All notable changes to the Go module and the `honk` CLI are documented here. The format follows [Keep a Changelog](https://keepachangelog.com/en/1.1.0/)
and the project uses [Semantic Versioning](https://semver.org/).

## [Unreleased]

### Changed
- The README and the `honk` CLI's help call the product Honk! Honk!; the module, the package
  and the command are unchanged.

## [0.4.0] - 2026-10-15

### Changed
- The module is now `github.com/honk-hq/go` (the package is still `honk`).

## [0.3.0] - 2026-10-14

### Added
- New names for the three main calls, with the same arguments and results:
  `c.Honk(ctx, m, opts...)` (was `Send`), `c.Uhoh(ctx, groupKey, title, message, opts...)`
  (was `Problem`) and `c.Phew(ctx, groupKey, title, message, opts...)` (was `Recovery`).
- The CLI is now called `honk`: `go install github.com/honk-me/honk-go/cmd/honk@latest`. Its
  subcommands `uhoh` and `phew` are the new names of `problem` and `recovery` (same flags,
  same request). Help, usage, error messages and `version` say `honk`, also when the CLI runs
  as `honk-me`: `honk-me version` prints `honk 0.3.0`.
- CLI: `honk uhoh GROUP_KEY [TITLE] MESSAGE` and `honk phew GROUP_KEY [TITLE] MESSAGE` (also
  as `problem`/`recovery`), e.g. `honk uhoh backup/db-1 "Backup failed" "Exit code 1"`. Two
  arguments are `GROUP_KEY MESSAGE`, three are `GROUP_KEY TITLE MESSAGE`; other flags can go
  anywhere. The `--group-key`/`--title`/`--message` form works as before; mixing it with the
  arguments is a usage error (exit 2).

### Changed
- `Send`, `Problem` and `Recovery` are now aliases of `Honk`, `Uhoh` and `Phew`. They are not
  deprecated and send byte-for-byte the same request, so existing code needs no change.
- `honk-me` stays as an alias of the CLI, so existing scripts keep working: `cmd/honk-me` is
  the same program (both commands are thin mains around `internal/cli`), and the `problem` and
  `recovery` subcommands still work. Release archives are now `honk_<version>_<os>_<arch>` and
  hold both binaries, `honk` and `honk-me`.
- The CLI's User-Agent suffix is now `honk-cli` (was `honk-me-cli`).
- README and godoc use the new names.

## [0.2.0] - 2026-10-07

### Added
- `Message.Actions`: up to 3 buttons (`honk.Action{Title, URL}`) with an `https://`,
  `mailto:`, `tel:` or `sms:` URL, and `WithActions` for the helpers. Validated locally like
  the server does, with errors named `actions[1].url`. Empty `Actions` are omitted, so
  messages without buttons are sent exactly as before.
- CLI: `--action TITLE=URL` (repeatable), e.g. `--action "Call Emily=tel:+15550134"`.

## [0.1.0] - 2026-10-04

### Added
- Package `honk`: context-aware `Client` for `POST /v1/messages` with every field of the v1
  ingestion API (including `ImageURL`), no dependencies outside the standard library.
- Automatic UUIDv7 `Idempotency-Key` (or `WithIdempotencyKey`), reused on every retry.
- Retries for network errors, timeouts, 429 and 5xx with exponential backoff, full jitter,
  `Retry-After` and a total deadline (a shorter context deadline wins).
- `*honk.Error` with `Kind`, plus `ErrValidation`, `ErrAuth`, `ErrQuota`, `ErrConflict`,
  `ErrNetwork`, `ErrTimeout`, `ErrServer` for `errors.Is`.
- Local validation of limits, enums and https-only URLs; `EncodeMessage` for the exact body.
- Helpers `Problem`, `Recovery`, `Info`, `Success`, `Warning`, `Error`, `Critical` with
  functional options; `FromEnv` / `OptionsFromEnv`.
- `honk-me` CLI (`send`, `problem`, `recovery`) for shell scripts, cron and CI, with
  documented exit codes, `--idempotency-key`, `--meta`, stdin messages and `--dry-run`.
- The Honk scale: constants `Light`, `Beep`, `Loud`, `Long`, `Blast` (aliases of the
  canonical severities), `ParseSeverity`, case-insensitive normalization before sending, the
  `Light`…`Blast` helpers, and CLI shortcuts `honk-me light|beep|loud|long|blast [TITLE] MESSAGE`.
