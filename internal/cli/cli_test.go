package cli

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"

	honk "github.com/honk-hq/go"
)

const testKey = "honk_ab12cd34ef56_0123456789abcdefghijABCDEFGHIJ0123"

type cliMock struct {
	*httptest.Server
	mu     sync.Mutex
	bodies []map[string]any
	keys   []string
}

func newCLIMock(t *testing.T, status int, body string, headers map[string]string) *cliMock {
	m := &cliMock{}
	m.Server = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		raw, _ := io.ReadAll(r.Body)
		var b map[string]any
		_ = json.Unmarshal(raw, &b)
		m.mu.Lock()
		m.bodies = append(m.bodies, b)
		m.keys = append(m.keys, r.Header.Get("Idempotency-Key"))
		m.mu.Unlock()
		for k, v := range headers {
			w.Header().Set(k, v)
		}
		w.WriteHeader(status)
		io.WriteString(w, body)
	}))
	t.Cleanup(m.Close)
	return m
}

const accepted = `{"id":"msg_1","status":"accepted","duplicate":false,"received_at":"2026-10-02T21:10:00.123Z"}`

func runCLI(t *testing.T, env map[string]string, in string, args ...string) (int, string, string) {
	t.Helper()
	var out, errOut bytes.Buffer
	stdout, stderr, stdin = &out, &errOut, strings.NewReader(in)
	code := run(context.Background(), args, func(k string) string { return env[k] })
	return code, out.String(), errOut.String()
}

func TestSendPrintsIDAndSerialisesFlags(t *testing.T) {
	m := newCLIMock(t, 202, accepted, nil)
	env := map[string]string{"HONK_URL": m.URL, "HONK_KEY": testKey, "HONK_SOURCE": "backup-host"}
	code, out, errOut := runCLI(t, env, "", "send", "--title", "Backup failed", "--message", "pg_dump exited with 1",
		"--severity", "error", "--priority", "high", "--category", "backups", "--channel", "nightly",
		"--group-key", "db/backup", "--event-type", "problem", "--url", "https://example.com/runs/1",
		"--image-url", "https://example.com/chart.png", "--meta", "host=db-1", "--meta", "exit:=1",
		"--ttl", "600", "--source-sequence", "9", "--occurred-at", "2026-10-02T21:10:00Z", "--idempotency-key", "backup-2026-10-02")
	if code != exitOK || out != "msg_1\n" {
		t.Fatalf("code %d out %q err %q", code, out, errOut)
	}
	want := `{"category":"backups","channel":"nightly","event_type":"problem","group_key":"db/backup","image_url":"https://example.com/chart.png","message":"pg_dump exited with 1","metadata":{"exit":1,"host":"db-1"},"occurred_at":"2026-10-02T21:10:00.000Z","priority":"high","severity":"error","source":"backup-host","source_sequence":9,"title":"Backup failed","ttl_seconds":600,"url":"https://example.com/runs/1"}`
	got, _ := json.Marshal(m.bodies[0])
	if string(got) != want {
		t.Fatalf("body\n got %s\nwant %s", got, want)
	}
	if m.keys[0] != "backup-2026-10-02" {
		t.Fatalf("key %q", m.keys[0])
	}
}

func TestActionFlags(t *testing.T) {
	m := newCLIMock(t, 202, accepted, nil)
	env := map[string]string{"HONK_URL": m.URL, "HONK_KEY": testKey}
	code, _, errOut := runCLI(t, env, "", "loud", "New request", "Emily asked for a quote",
		"--action", "Reply=mailto:emily@example.com?subject=Your%20quote", "--action", "Call Emily=tel:+15550134")
	if code != exitOK {
		t.Fatalf("code %d err %q", code, errOut)
	}
	got, _ := json.Marshal(m.bodies[0]["actions"])
	if want := `[{"title":"Reply","url":"mailto:emily@example.com?subject=Your%20quote"},{"title":"Call Emily","url":"tel:+15550134"}]`; string(got) != want {
		t.Fatalf("actions\n got %s\nwant %s", got, want)
	}
	if code, _, e := runCLI(t, env, "", "send", "--message", "x", "--action", "Run=javascript:alert(1)"); code != exitInvalid || !strings.Contains(e, "actions[0].url") {
		t.Errorf("invalid action: %d %q", code, e)
	}
	if len(m.bodies) != 1 {
		t.Errorf("an invalid action was sent: %v", m.bodies)
	}
}

func TestProblemRecoveryAndStdin(t *testing.T) {
	m := newCLIMock(t, 202, accepted, nil)
	env := map[string]string{"HONK_URL": m.URL, "HONK_KEY": testKey, "HONK_IDEMPOTENCY_KEY": "ci-run-7"}
	if code, _, e := runCLI(t, env, "line 1\nline 2\n", "problem", "--group-key", "ci/deploy", "--title", "Deploy failed", "--message", "-", "--quiet"); code != 0 {
		t.Fatalf("problem: %d %s", code, e)
	}
	if code, out, e := runCLI(t, env, "", "recovery", "--group-key", "ci/deploy", "--message", "green again", "--json"); code != 0 || !strings.Contains(out, `"id":"msg_1"`) {
		t.Fatalf("recovery: %d %q %s", code, out, e)
	}
	if m.bodies[0]["event_type"] != "problem" || m.bodies[0]["severity"] != "error" || m.bodies[0]["message"] != "line 1\nline 2" {
		t.Fatalf("problem body %v", m.bodies[0])
	}
	if m.bodies[1]["event_type"] != "recovery" || m.bodies[1]["severity"] != "success" {
		t.Fatalf("recovery body %v", m.bodies[1])
	}
	if m.keys[0] != "ci-run-7" {
		t.Fatalf("HONK_IDEMPOTENCY_KEY not used: %q", m.keys[0])
	}
}

func TestExitCodes(t *testing.T) {
	cases := []struct {
		name    string
		status  int
		body    string
		headers map[string]string
		want    int
		stderr  string
	}{
		{"validation", 422, `{"error":{"code":"validation_failed","message":"Request validation failed","fields":[{"field":"x","code":"invalid_enum"}]}}`, nil, exitInvalid, "invalid_enum"},
		{"auth", 401, `{"error":{"code":"invalid_key","message":"Invalid or revoked ingestion key"}}`, nil, exitAuth, "invalid_key"},
		{"urgent", 403, `{"error":{"code":"priority_not_allowed","message":"no urgent"}}`, nil, exitAuth, "priority_not_allowed"},
		{"quota", 429, `{"error":{"code":"quota_exceeded","message":"Daily quota"}}`, map[string]string{"Retry-After": "3600"}, exitQuota, "Retry-After 1h0m0s"},
		{"conflict", 409, `{"error":{"code":"idempotency_conflict","message":"different payload"}}`, nil, exitConflict, "idempotency_conflict"},
		{"server", 503, `{"error":{"code":"unavailable","message":"paused"}}`, map[string]string{"Retry-After": "60"}, exitTemporary, "--idempotency-key"},
		{"notfound", 404, `404 page not found`, nil, exitError, "base address"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			m := newCLIMock(t, tc.status, tc.body, tc.headers)
			code, _, errOut := runCLI(t, map[string]string{"HONK_URL": m.URL, "HONK_KEY": testKey}, "", "send", "--message", "x", "--retries", "0")
			if code != tc.want || !strings.Contains(errOut, tc.stderr) {
				t.Fatalf("code %d (want %d), stderr %q (want %q)", code, tc.want, errOut, tc.stderr)
			}
		})
	}
}

func TestUsageErrors(t *testing.T) {
	env := map[string]string{"HONK_URL": "https://honk.example.com", "HONK_KEY": testKey}
	for _, tc := range []struct {
		args []string
		env  map[string]string
		want int
		msg  string
	}{
		{nil, env, exitUsage, "Usage"},
		{[]string{"shout"}, env, exitUsage, "unknown command"},
		{[]string{"send"}, env, exitUsage, "--message is required"},
		{[]string{"send", "--message", "x", "extra"}, env, exitUsage, "unexpected argument"},
		{[]string{"problem", "--message", "x"}, env, exitUsage, "needs --group-key"},
		{[]string{"send", "--message", "x", "--occurred-at", "yesterday"}, env, exitUsage, "RFC 3339"},
		{[]string{"send", "--message", "x", "--meta", "novalue"}, env, exitUsage, "key=value"},
		{[]string{"send", "--message", "x", "--action", "tel:+15550134"}, env, exitUsage, "TITLE=URL"},
		{[]string{"send", "--message", "x"}, map[string]string{"HONK_KEY": testKey}, exitUsage, "HONK_URL"},
		{[]string{"send", "--message", "x"}, map[string]string{"HONK_URL": "https://h"}, exitUsage, "HONK_KEY"},
		{[]string{"send", "--message", "x", "--severity", "fatal"}, env, exitInvalid, "severity must be one of"},
	} {
		code, _, errOut := runCLI(t, tc.env, "", tc.args...)
		if code != tc.want || !strings.Contains(errOut, tc.msg) {
			t.Errorf("%v: code %d (want %d) stderr %q (want %q)", tc.args, code, tc.want, errOut, tc.msg)
		}
	}
}

func TestDryRunAndVersion(t *testing.T) {
	code, out, _ := runCLI(t, nil, "", "send", "--dry-run", "--message", "x", "--meta", "n:=2", "--title", "A & B")
	if code != 0 || !strings.Contains(out, `"title": "A & B"`) || !strings.Contains(out, `"n": 2`) {
		t.Fatalf("dry run %d %q", code, out)
	}
	code, out, _ = runCLI(t, nil, "", "version")
	if code != 0 || out != "honk "+honk.Version+"\n" {
		t.Fatalf("version %d %q", code, out)
	}
}

// uhoh and phew are the new names of problem and recovery: same flags, same request.
func TestUhohPhewAreProblemRecovery(t *testing.T) {
	m := newCLIMock(t, 202, accepted, nil)
	env := map[string]string{"HONK_URL": m.URL, "HONK_KEY": testKey, "HONK_SOURCE": "oven-pi"}
	flags := func(key string) []string {
		return []string{"--group-key", "oven/kitchen", "--title", "The cake is burning", "--message", "-", "--channel", "home",
			"--meta", "celsius:=260", "--source-sequence", "3", "--idempotency-key", key, "--quiet"}
	}
	for _, pair := range [][2]string{{"uhoh", "problem"}, {"phew", "recovery"}} {
		for _, cmd := range pair {
			if code, out, e := runCLI(t, env, "Oven at 260 °C\n", append([]string{cmd}, flags(pair[0])...)...); code != exitOK || out != "" {
				t.Fatalf("%s: %d %q %s", cmd, code, out, e)
			}
		}
	}
	if len(m.bodies) != 4 {
		t.Fatalf("%d requests", len(m.bodies))
	}
	for i, want := range []string{"problem", "recovery"} {
		fresh, old := m.bodies[2*i], m.bodies[2*i+1]
		a, _ := json.Marshal(fresh)
		b, _ := json.Marshal(old)
		if string(a) != string(b) || m.keys[2*i] != m.keys[2*i+1] {
			t.Errorf("%s: new and old name differ\n new %s key %s\n old %s key %s", want, a, m.keys[2*i], b, m.keys[2*i+1])
		}
		if fresh["event_type"] != want || fresh["group_key"] != "oven/kitchen" || fresh["message"] != "Oven at 260 °C" {
			t.Errorf("%s body %s", want, a)
		}
	}
	if m.bodies[0]["severity"] != "error" || m.bodies[2]["severity"] != "success" {
		t.Errorf("default severities: %v / %v", m.bodies[0]["severity"], m.bodies[2]["severity"])
	}
	if code, _, e := runCLI(t, env, "", "phew", "--message", "x", "--severity", "light", "--group-key", "g", "--dry-run"); code != exitOK || e != "" {
		t.Errorf("phew --severity: %d %q", code, e)
	}
	for _, cmd := range []string{"uhoh", "phew"} {
		if code, _, e := runCLI(t, env, "", cmd, "--message", "x"); code != exitUsage || e != "honk: "+cmd+" needs --group-key\n" {
			t.Errorf("%s without --group-key: %d %q", cmd, code, e)
		}
	}
}

// uhoh/phew (and problem/recovery) take GROUP_KEY [TITLE] MESSAGE as arguments: two are
// GROUP_KEY MESSAGE, three GROUP_KEY TITLE MESSAGE. Other flags go anywhere; mixing the
// arguments with --group-key, --title or --message is refused.
func TestUhohPhewPositional(t *testing.T) {
	m := newCLIMock(t, 202, accepted, nil)
	env := map[string]string{"HONK_URL": m.URL, "HONK_KEY": testKey}
	cases := []struct {
		args []string
		in   string
		want map[string]any
	}{
		{[]string{"uhoh", "backup/db-1", "Backup failed", "Exit code 1"},
			"", map[string]any{"title": "Backup failed", "message": "Exit code 1", "severity": "error", "group_key": "backup/db-1", "event_type": "problem"}},
		{[]string{"phew", "backup/db-1", "Backup is back", "All good"},
			"", map[string]any{"title": "Backup is back", "message": "All good", "severity": "success", "group_key": "backup/db-1", "event_type": "recovery"}},
		{[]string{"uhoh", "oven/kitchen", "The cake is burning"},
			"", map[string]any{"message": "The cake is burning", "severity": "error", "group_key": "oven/kitchen", "event_type": "problem"}},
		{[]string{"phew", "oven/kitchen", "--severity", "light", "Cake saved", "--source", "oven-pi"},
			"", map[string]any{"message": "Cake saved", "severity": "info", "source": "oven-pi", "group_key": "oven/kitchen", "event_type": "recovery"}},
		{[]string{"uhoh", "--channel", "home", "oven/kitchen", "Smoke", "-", "--source-sequence", "4"},
			"Oven at 260 °C\n", map[string]any{"title": "Smoke", "message": "Oven at 260 °C", "severity": "error", "channel": "home", "group_key": "oven/kitchen", "event_type": "problem", "source_sequence": 4}},
		{[]string{"problem", "backup/db-1", "Backup failed", "Exit code 1"},
			"", map[string]any{"title": "Backup failed", "message": "Exit code 1", "severity": "error", "group_key": "backup/db-1", "event_type": "problem"}},
		{[]string{"recovery", "backup/db-1", "All good"},
			"", map[string]any{"message": "All good", "severity": "success", "group_key": "backup/db-1", "event_type": "recovery"}},
	}
	for i, tc := range cases {
		code, out, errOut := runCLI(t, env, tc.in, tc.args...)
		if code != exitOK || out != "msg_1\n" {
			t.Fatalf("%v: code %d out %q err %q", tc.args, code, out, errOut)
		}
		got, _ := json.Marshal(m.bodies[i])
		want, _ := json.Marshal(tc.want)
		if string(got) != string(want) {
			t.Errorf("%v\n got %s\nwant %s", tc.args, got, want)
		}
	}

	// The arguments and the flags build the very same request.
	n := len(m.bodies)
	for _, args := range [][]string{
		{"uhoh", "backup/db-1", "Backup failed", "Exit code 1", "--idempotency-key", "k-1"},
		{"uhoh", "--group-key", "backup/db-1", "--title", "Backup failed", "--message", "Exit code 1", "--idempotency-key", "k-1"},
		{"phew", "backup/db-1", "All good", "--idempotency-key", "k-2"},
		{"phew", "--group-key", "backup/db-1", "--message", "All good", "--idempotency-key", "k-2"},
	} {
		if code, _, e := runCLI(t, env, "", args...); code != exitOK {
			t.Fatalf("%v: %d %s", args, code, e)
		}
	}
	for i := n; i < n+4; i += 2 {
		a, _ := json.Marshal(m.bodies[i])
		b, _ := json.Marshal(m.bodies[i+1])
		if string(a) != string(b) || m.keys[i] != m.keys[i+1] {
			t.Errorf("arguments and flags differ\n args  %s %s\n flags %s %s", a, m.keys[i], b, m.keys[i+1])
		}
	}

	sent := len(m.bodies)
	for _, tc := range []struct {
		args []string
		msg  string
	}{
		{[]string{"uhoh", "backup/db-1", "Exit code 1", "--message", "x"}, "honk: uhoh takes either GROUP_KEY [TITLE] MESSAGE or --group-key/--title/--message, not both (got arguments and --message)\n"},
		{[]string{"phew", "--group-key", "a", "b", "c"}, "honk: phew takes either GROUP_KEY [TITLE] MESSAGE or --group-key/--title/--message, not both (got arguments and --group-key)\n"},
		{[]string{"problem", "g", "t", "m", "--title", "x", "--group-key", "h"}, "(got arguments and --group-key, --title)\n"},
		{[]string{"uhoh", "Backup failed"}, "honk: uhoh takes GROUP_KEY [TITLE] MESSAGE (quote them), got 1 argument(s)\n"},
		{[]string{"phew", "a", "b", "c", "d"}, "honk: phew takes GROUP_KEY [TITLE] MESSAGE (quote them), got 4 argument(s)\n"},
		{[]string{"uhoh", "", "Exit code 1"}, "honk: uhoh needs --group-key\n"},
		{[]string{"send", "backup/db-1", "x"}, `honk: unexpected argument "backup/db-1" (use --message)`},
	} {
		code, _, e := runCLI(t, env, "", tc.args...)
		if code != exitUsage || (!strings.HasSuffix(e, tc.msg) && !strings.HasPrefix(e, tc.msg)) {
			t.Errorf("%v: code %d stderr %q (want %q)", tc.args, code, e, tc.msg)
		}
	}
	if len(m.bodies) != sent {
		t.Errorf("a refused command was sent: %v", m.bodies[sent:])
	}
}

func TestHelpShowsHonk(t *testing.T) {
	code, out, _ := runCLI(t, nil, "", "help")
	if code != exitOK {
		t.Fatalf("help: %d", code)
	}
	for _, want := range []string{"honk " + honk.Version + " — send an event to Honk", "\n  honk uhoh GROUP_KEY [TITLE] MESSAGE [flags]",
		"\n  honk phew GROUP_KEY [TITLE] MESSAGE [flags]", "two arguments are GROUP_KEY MESSAGE, three are GROUP_KEY TITLE MESSAGE",
		"--group-key KEY [--title TEXT] --message TEXT instead of the arguments (not both)", "\n  honk send", `Run "honk send -h"`} {
		if !strings.Contains(out, want) {
			t.Errorf("help lacks %q:\n%s", want, out)
		}
	}
	_, _, e := runCLI(t, nil, "", "uhoh", "-h")
	if !strings.HasPrefix(e, "Usage: honk uhoh GROUP_KEY [TITLE] MESSAGE [flags]\n   or: honk uhoh --group-key KEY [--title TEXT] --message TEXT [flags]\n") ||
		!strings.Contains(e, "uhoh/phew need one: this flag or the GROUP_KEY argument") {
		t.Errorf("uhoh -h: %q", e)
	}
	if _, _, e := runCLI(t, nil, "", "recovery", "-h"); !strings.HasPrefix(e, "Usage: honk recovery GROUP_KEY [TITLE] MESSAGE [flags]\n") {
		t.Errorf("recovery -h: %q", e)
	}
	if _, _, e := runCLI(t, nil, "", "send", "-h"); !strings.HasPrefix(e, "Usage: honk send [flags]\n") {
		t.Errorf("send -h: %q", e)
	}
	_, _, e = runCLI(t, nil, "", "loud", "-h")
	if !strings.HasPrefix(e, "Usage: honk loud [TITLE] MESSAGE [flags]") || !strings.Contains(e, "| honk send --message -") {
		t.Errorf("loud -h: %q", e)
	}
	if code, _, e := runCLI(t, nil, "", "shout"); code != exitUsage || !strings.HasPrefix(e, `honk: unknown command "shout"`) {
		t.Errorf("unknown command: %d %q", code, e)
	}
}

func TestHornShortcutSubcommands(t *testing.T) {
	m := newCLIMock(t, 202, accepted, nil)
	env := map[string]string{"HONK_URL": m.URL, "HONK_KEY": testKey}
	cases := []struct {
		args []string
		want map[string]any
	}{
		{[]string{"loud", "Disk 91%"}, map[string]any{"message": "Disk 91%", "severity": "warning"}},
		{[]string{"blast", "Payments down", "Stripe answers 500", "--channel", "payments"}, map[string]any{"title": "Payments down", "message": "Stripe answers 500", "severity": "critical", "channel": "payments"}},
		{[]string{"beep", "--group-key", "deploy/web", "Deployed v4.2"}, map[string]any{"message": "Deployed v4.2", "severity": "success", "group_key": "deploy/web"}},
		{[]string{"light", "--message", "from a flag"}, map[string]any{"message": "from a flag", "severity": "info"}},
		{[]string{"long", "Backup failed", "exit 1"}, map[string]any{"title": "Backup failed", "message": "exit 1", "severity": "error"}},
		{[]string{"send", "--message", "x", "--severity", "LOUD"}, map[string]any{"message": "x", "severity": "warning"}},
		{[]string{"problem", "--group-key", "g", "--message", "x", "--severity", "blast"}, map[string]any{"message": "x", "severity": "critical", "group_key": "g", "event_type": "problem"}},
	}
	for i, tc := range cases {
		code, out, errOut := runCLI(t, env, "", tc.args...)
		if code != exitOK || out != "msg_1\n" {
			t.Fatalf("%v: code %d out %q err %q", tc.args, code, out, errOut)
		}
		got, _ := json.Marshal(m.bodies[i])
		want, _ := json.Marshal(tc.want)
		if string(got) != string(want) {
			t.Errorf("%v\n got %s\nwant %s", tc.args, got, want)
		}
	}
	if code, _, e := runCLI(t, env, "", "loud", "a", "b", "c"); code != exitUsage || !strings.Contains(e, "[TITLE] MESSAGE") {
		t.Errorf("three positionals: %d %q", code, e)
	}
	if code, _, e := runCLI(t, env, "", "loud"); code != exitUsage || !strings.Contains(e, "--message is required") {
		t.Errorf("no message: %d %q", code, e)
	}
	if code, _, e := runCLI(t, env, "", "loud", "x", "--severity", "light"); code != exitUsage {
		t.Errorf("--severity on a shortcut should be unknown: %d %q", code, e)
	}
}
