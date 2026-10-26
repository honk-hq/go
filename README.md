# honk-go

[![CI](https://github.com/honk-hq/go/actions/workflows/ci.yml/badge.svg)](https://github.com/honk-hq/go/actions/workflows/ci.yml)
[![Go Reference](https://pkg.go.dev/badge/github.com/honk-hq/go.svg)](https://pkg.go.dev/github.com/honk-hq/go)

Official Go client and CLI for [Honk! Honk!](https://honk-honk.app), the inbox that turns events from
your apps, scripts, cron jobs and CI into calm, grouped push notifications on your phone.

```go
c, err := honk.FromEnv() // HONK_URL and HONK_KEY
if err != nil {
	log.Fatal(err)
}
c.Uhoh(ctx, "oven/kitchen", "The cake is burning", "Oven at 260 °C. The smoke alarm agrees.")
// …a few tense minutes later
c.Phew(ctx, "oven/kitchen", "Cake saved", "Nobody needs to know.")
```

Or from a shell, cron or CI:

```sh
pg_dump app > /backup/app.sql || honk uhoh backup/db-1 "Backup failed" "Exit code $?"
honk beep "Someone bought 3 rubber chickens" "Order 1042, €2.97"
```

`Uhoh` opens an incident for its group key and your phone honks. If the oven keeps
complaining, the same notification is updated instead of honking again. `Phew` closes the
incident, and `Honk` sends any other message.

Until 0.3.0 the methods were called `Send`, `Problem` and `Recovery`, and the CLI's subcommands
`problem` and `recovery`. All of them still work and send exactly the same requests.

- Package `honk`: context-aware, standard library only, safe for concurrent use.
- CLI `honk` for shell scripts, cron and CI (replaces the cURL snippet).
- Retries with backoff, `Retry-After`, a total deadline and an idempotency key on every send,
  so a retry never creates a duplicate.
- A thin wrapper: the server checks your message, delivers it and tells you what it corrected
  (`Accepted.Warnings`, see [Warnings](#warnings)).

The ingestion key (`honk_…`) is a secret: keep it in the environment or a secret store, never
in source code or client apps.

## Install

```sh
go get github.com/honk-hq/go                       # library (Go 1.22+)
go install github.com/honk-hq/go/cmd/honk@latest   # CLI
```

The CLI is also attached as prebuilt binaries (Linux, macOS, Windows; amd64 and arm64) to
every [GitHub release](https://github.com/honk-hq/go/releases).

Create a project and an ingestion key at [honk-honk.app](https://honk-honk.app). Its
*Integrations* page generates ready-to-paste code for the library and the CLI.

## Quick start

```go
import honk "github.com/honk-hq/go" // package honk

c, err := honk.FromEnv() // HONK_URL, HONK_KEY (+ optional HONK_SOURCE, HONK_ENVIRONMENT, HONK_CHANNEL)
if err != nil {
	log.Fatal(err)
}
_, err = c.Beep(ctx, "Backup finished", "nightly pg_dump took 42 s")
```

Or explicitly: `honk.New(honk.Options{URL: "https://honk.example.com", Key: key})`.

## The Honk scale

Five levels. The Honk app shows them by their standard names (Info, Success, Warning, Error,
Critical); people who prefer horn sounds can switch on the Honk names (Light, Beep, Loud, Long,
Blast) with the "Honk names for levels" switch in Settings. The SDK takes both: the canonical values and the
Honk names as input aliases, in any case, and always sends the canonical value.

| Level | Honk name | Severity | Method | Constant | CLI |
|---|---|---|---|---|---|
| Info | Light | `light` (info) | `c.Light(ctx, title, message, opts...)` | `honk.Light` | `honk light …` |
| Success | Beep | `beep` (success) | `c.Beep(…)` | `honk.Beep` | `honk beep …` |
| Warning | Loud | `loud` (warning) | `c.Loud(…)` | `honk.Loud` | `honk loud …` |
| Error | Long | `long` (error) | `c.Long(…)` | `honk.Long` | `honk long …` |
| Critical | Blast | `blast` (critical) | `c.Blast(…)` | `honk.Blast` | `honk blast …` |

The horn constants are aliases (`honk.Loud == honk.SeverityWarning`), and `Severity: "LOUD"`
is normalized too, so alias and canonical spellings are the same event, also for idempotency.
The level alone decides how loudly the push interrupts. `Info`, `Success`, `Warning`, `Error` and
`Critical` remain as synonyms; `honk.ParseSeverity("Blast")` returns `SeverityCritical`.

## Recipe: notify me when a customer asks for something

One group per request (`requests/<id>`) and a stable idempotency key: two different customers
never fold into one notification, and a retried handler or job never buzzes twice.

```go
var notifier, _ = honk.FromEnv() // create once, reuse (keep-alive)

func onCustomerRequest(r CustomerRequest) {
	// ...save the request first, then notify off the request path:
	go func() {
		ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
		defer cancel()
		_, err := notifier.Honk(ctx, honk.Message{
			Title:    "New request: " + r.Subject, // too long? the server shortens it (a warning)
			Message:  fmt.Sprintf("%s (%s) asked: %s", r.Name, r.Company, r.Body),
			Severity: honk.Loud,
			Category: honk.CategoryCustomers,
			Channel:  "requests",
			GroupKey: fmt.Sprintf("requests/%d", r.ID), // one group per request
			URL:      fmt.Sprintf("https://shop.example.com/admin/requests/%d", r.ID),
			Metadata: map[string]any{"request_id": strconv.Itoa(r.ID)},
		}, honk.WithIdempotencyKey(fmt.Sprintf("request-%d", r.ID)))
		if err != nil {
			log.Printf("honk: %v", err) // never fail the customer's request
		}
	}()
}
```

From a queue worker, return the error when `he.Retryable()` and let the queue retry with the
same key.

## Grouping in three lines

Messages with the same `GroupKey` (per project, environment, source and channel) form one
group: the first one pushes, repeats update it calmly instead of buzzing again. Use one key
per customer request (`requests/<id>`), and a shared key only for repeats of the same problem
(`queue/failed-jobs`). The group key is optional: without one a message groups by its title.
Give `Uhoh`/`Phew` pairs the same key so the recovery closes the right incident.

## Sending

```go
func (c *Client) Honk(ctx context.Context, m honk.Message, opts ...honk.Option) (*honk.Accepted, error)
// Accepted{ID, Duplicate, ReceivedAt, Warnings}
```

The essentials:

| Field | Notes |
|---|---|
| `Message` | the text, line breaks allowed |
| `Title` | one line; default: the first line of `Message`. A title or a message is required |
| `Severity` | the level: `honk.Light` `Beep` `Loud` `Long` `Blast` (or `SeverityInfo` … `SeverityCritical`, or any-case strings); default Light |
| `GroupKey` | optional; relates occurrences (see above) |
| `EventType` | `EventTypeProblem` / `EventTypeRecovery`, or use `Uhoh` / `Phew` |

Also: `Priority`, `Category`, `Source` / `Environment` / `Channel` (default `Options.Defaults`),
`OccurredAt`, `URL`, `ImageURL`, `Actions`, `Metadata`, `TTLSeconds`, `SourceSequence`
(`honk.Ptr[int64](n)`). The SDK sends them as given; the server applies the limits and says
what it corrected (see [Warnings](#warnings)).

Zero values are omitted. A nil error means Honk! Honk! **durably stored** the message (`202`), not
that a push was delivered or read. `honk.EncodeMessage(m, defaults)` returns the exact JSON.

Helpers take the core fields plus options (`WithIdempotencyKey`, `WithSeverity`,
`WithPriority`, `WithCategory`, `WithSource`, `WithEnvironment`, `WithChannel`,
`WithGroupKey`, `WithOccurredAt`, `WithURL`, `WithImageURL`, `WithActions`, `WithMetadata`,
`WithTTLSeconds`, `WithSourceSequence`):

```go
c.Loud(ctx, "Disk 91%", "/var on app-01", honk.WithGroupKey("disk/app-01/var"))
c.Light(ctx, "Deploy started", "v4.2.0", honk.WithChannel("deploys"))
c.Beep(...); c.Long(...); c.Blast(...)
c.Uhoh(ctx, "db/backup", "Backup failed", "pg_dump exited with 1")   // event_type problem, a Long message by default
c.Phew(ctx, "db/backup", "Backup OK", "pg_dump finished in 41 s")    // event_type recovery, a Beep message by default
```

### Buttons (actions)

Up to three buttons on the message, in display order: reply to the customer, call them, open
the order.

```go
_, err := c.Honk(ctx, honk.Message{
	Title:    "New request: online shop quote",
	Message:  "Maria Tudor (Acme) asked for a quote: online shop, 40 products",
	Category: honk.CategoryCustomers,
	GroupKey: "requests/4812",
	Actions: []honk.Action{
		{Title: "Reply", URL: "mailto:maria@example.com?subject=" + url.PathEscape("Your quote")},
		{Title: "Call Maria", URL: "tel:+15550134"},
	},
})
```

- `Title`: a short label, shown as sent.
- `URL`: `https://`, `mailto:` (optionally `?subject=…&body=…`, percent-encoded with
  `url.PathEscape`), `tel:` or `sms:`. The server leaves out a button it cannot use and keeps
  the others, with a warning that names it: `actions[1].url`.
- Honk! Honk! never opens or fetches them; the app does when you tap one.
- Helpers take `honk.WithActions(honk.Action{…}, …)`; the CLI takes
  `--action "Call Maria=tel:+15550134"`.

### Options

```go
honk.New(honk.Options{
	URL:      "https://honk.example.com",
	Key:      key,
	Timeout:  5 * time.Second,  // per attempt
	Retries:  4,                // after the first attempt; honk.NoRetries disables
	Deadline: 30 * time.Second, // total, waits included; a shorter ctx deadline wins
	Defaults: honk.Defaults{Source: "billing", Environment: "production"},
	Logger:   slog.Default(),   // logs the server's warnings (optional)
	HTTPClient: myClient,       // proxies, tracing; redirects are never followed
})
```

## Warnings

Honk! Honk! delivers first: an alert is never lost to an optional field. The server refuses
only what it cannot deliver (no title and no message, an unknown field or event type, a bad
key, …) and corrects everything else: it shortens a title that is too long, leaves out an
invalid URL or button, moves a TTL into range, lowers `urgent` to `high` for a key without
*allow urgent*. The SDK does not check field contents itself; it returns what the server
corrected:

```go
acc, err := c.Honk(ctx, m)
if err == nil {
	for _, w := range acc.Warnings { // []honk.Warning{Field, Code, Message}
		log.Printf("honk: %s", w) // "title truncated: at most 160 characters; shortened"
	}
}
```

`Code` is one of `honk.WarningTruncated`, `WarningNormalized`, `WarningIgnored`,
`WarningClamped`, `WarningDowngraded` and `WarningHashed`. Warnings are never errors: the
message was accepted. Set `Options.Logger` (a `*slog.Logger`) to log every warning at `Warn`
level instead. The CLI prints them to stderr (`honk: warning: …`).

## Retries and idempotency, guaranteed

- Every call carries an `Idempotency-Key`: yours (`WithIdempotencyKey`), or a fresh UUIDv7
  (`honk.NewIdempotencyKey()`). **The same key is reused on every retry.** Within 24 h Honk! Honk!
  answers a replay with the original ID and `Duplicate: true`, so a lost response never creates
  a second message.
- Only network errors, timeouts, `429` and `5xx` are retried, with exponential backoff and
  full jitter (`rand(0, min(8 s, 0.5 s·2ⁿ))`), never sooner than the server's `Retry-After`.
- Everything stops at the deadline (or the context's): if the next wait would cross it (for
  example a daily quota that resets at midnight), the error is returned at once with
  `RetryAfter`.
- `4xx` other than `429` are never retried: fix the request instead.
- Short per-attempt timeouts and a keep-alive transport: create one `Client` and share it.

## Errors

Errors are `*honk.Error` (`Kind`, `Status`, `Code`, `Message`, `Fields`, `Local`,
`RequestID`, `IdempotencyKey`, `Attempts`, `RetryAfter`, `Retryable()`), and match sentinels
with `errors.Is`. `err.Error()` carries what the server said, the refused fields and the
request ID:

```
honk: 422 validation_failed: Request validation failed (event_type invalid_enum: must be one of event, problem, recovery) [request req_01k6…]
```

| Sentinel / Kind | When | What to do |
|---|---|---|
| `ErrValidation` / `KindValidation` | `400`/`413`/`415`/`422` (no title or message, invalid JSON, over 16 KiB, unknown field or event type), or `Local`: nothing to send | fix the message |
| `ErrAuth` / `KindAuth` | `401 invalid_key`, `403 project_suspended`, `workspace_suspended` | fix the key |
| `ErrQuota` / `KindQuota` | `429 quota_exceeded` (daily, resets at UTC midnight) or `rate_limited`, after retries | retry after `RetryAfter` |
| `ErrConflict` / `KindConflict` | `409 idempotency_conflict`: same key, different payload | new key or original payload |
| `ErrNetwork` / `KindNetwork` | unreachable on every attempt | retry later, same key |
| `ErrTimeout` / `KindTimeout` | attempts (or the context) timed out; maybe stored | retry later, same key |
| `ErrServer` / `KindServer` | `5xx` on every attempt | retry later, same key |

`KindCanceled` wraps `context.Canceled`; timeouts also match `context.DeadlineExceeded` when
the context expired.

```go
_, err := c.Long(ctx, "Payment failed", "Stripe declined order 1042", honk.WithGroupKey("payments/stripe"))
var he *honk.Error
switch {
case err == nil:
case errors.Is(err, honk.ErrValidation):
	log.Printf("bug: %v", err) // he.Fields says what to fix
case errors.As(err, &he) && he.Retryable():
	requeue(he.IdempotencyKey, he.RetryAfter)
default:
	log.Print(err)
}
```

## CLI: `honk`

For shell scripts, cron and CI. Reads `HONK_URL` and `HONK_KEY` (never pass the key as a
flag), plus optional `HONK_SOURCE`, `HONK_ENVIRONMENT`, `HONK_CHANNEL` and
`HONK_IDEMPOTENCY_KEY`.

```sh
honk loud "Disk 91%"                                   # shortcut: light, beep, loud, long, blast
honk beep "Backup finished" "nightly pg_dump took 42 s" # [TITLE] MESSAGE, flags anywhere
honk send --title "Disk almost full" --message "/var at 91%" --severity loud \
  --group-key "disk/$(hostname)/var" --source "$(hostname)" --meta host="$(hostname)" --meta used:=91
honk uhoh backup/db-1 "Backup failed" "Exit code 1"    # GROUP_KEY [TITLE] MESSAGE
honk phew backup/db-1 "Backup is back" "All good"
honk phew --group-key backup/db-1 --title "Backup is back" --message "All good"   # the same, with flags
tail -c 8000 /var/log/backup.log | honk send --title "Backup log" --message -   # message from stdin
honk send --message "Front door" --image-url https://cam.example.com/snap.jpg --priority high
honk loud "Disk 91%" "/var on app-01" --action "Open Grafana=https://grafana.example.com/d/disk"
```

Cron, alerting only when the job fails:

```cron
0 3 * * * pg_dump app > /backup/app.sql || honk uhoh --group-key db/backup --source "$(hostname)" --title "Backup failed" --message "pg_dump exited with $?"
```

Send `phew` only when something was actually broken (for example from a check that
remembers its last state): a recovery with no open problem still opens a "recovered" episode
and notifies, so a `phew` after every successful run would buzz every night.

CI (GitHub Actions), with one key per run attempt so the CLI's own retries never duplicate:

```yaml
- name: Notify
  if: failure()
  env:
    HONK_URL: ${{ secrets.HONK_URL }}
    HONK_KEY: ${{ secrets.HONK_KEY }}
  run: |
    honk uhoh --group-key "ci/${{ github.repository }}/${{ github.ref_name }}" \
      --title "CI failed: ${{ github.workflow }}" --message "${{ github.server_url }}/${{ github.repository }}/actions/runs/${{ github.run_id }}" \
      --source github-actions --category deployments \
      --idempotency-key "gh-${{ github.run_id }}-${{ github.run_attempt }}" || true
```

Shortcuts `honk light|beep|loud|long|blast [TITLE] MESSAGE [flags]` fix the severity and
take the text as arguments. `honk uhoh|phew GROUP_KEY [TITLE] MESSAGE [flags]` take the group
key and the text as arguments: two arguments are `GROUP_KEY MESSAGE`, three are
`GROUP_KEY TITLE MESSAGE`. Or give them as `--group-key`, `--title` and `--message`, but not
both ways at once (that is a usage error, exit 2). `send` takes flags only. The flags:
`--title`, `--message` (`-` = stdin), `--severity` (horn or canonical name), `--priority`, `--category`,
`--source`, `--environment`, `--channel`, `--group-key`, `--event-type` (send only),
`--occurred-at` (RFC 3339 or `now`), `--url`, `--image-url`, `--action TITLE=URL` (repeatable,
up to 3; split at the first `=`), `--meta k=v` / `--meta k:=3` (repeatable), `--ttl`,
`--source-sequence`, `--idempotency-key`, `--timeout`, `--deadline`, `--retries`, `--json`,
`--quiet`, `--dry-run`. `honk send -h` lists them all.

On success it prints the message ID (or JSON with `--json`, including `warnings`), and the
server's warnings to stderr as `honk: warning: …` (`--quiet` prints nothing). The group key of
`uhoh`/`phew` is optional with the flags. Exit codes:

| Code | Meaning |
|---|---|
| 0 | accepted (or a duplicate of an accepted event) |
| 1 | unexpected answer (wrong `HONK_URL`, redirect, malformed reply) |
| 2 | usage error, or `HONK_URL` / `HONK_KEY` missing |
| 3 | refused by the server: fix the flags (stderr names the fields) |
| 4 | authentication: invalid/revoked key, suspended project |
| 5 | quota or rate limit (`429`); stderr shows `Retry-After` |
| 6 | idempotency conflict (`409`): same key, different payload |
| 7 | temporary failure (network, timeout, `5xx`) after retries: rerun with the same `--idempotency-key` |

Add `|| true` where a notification failure must not fail the script.

## Development

```sh
go test -race ./...                                   # unit tests (httptest) + CLI tests
HONK_URL=… HONK_KEY=… go test -run Integration ./...  # against a real server (use a test project's key)
```

The version lives in `honk.Version` (also the User-Agent and `honk version`). Releases: push
a tag `vX.Y.Z` matching it; the release workflow attaches the CLI binaries and the Go proxy
serves the module (see `CHANGELOG.md`).

## Links

- [honk-honk.app](https://honk-honk.app): the Honk! Honk! inbox (web, iPhone).
- Other SDKs: [Node.js](https://github.com/honk-hq/node),
  [PHP / Laravel](https://github.com/honk-hq/php), [Swift](https://github.com/honk-hq/honk-swift),
  [Kotlin / Java](https://github.com/honk-hq/kotlin).

MIT License.
