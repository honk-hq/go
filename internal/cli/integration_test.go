package cli

// Runs the CLI against a real Honk server when HONK_URL and HONK_KEY are set; skipped otherwise.

import (
	"fmt"
	"os"
	"strings"
	"testing"
	"time"
)

func TestIntegrationCLI(t *testing.T) {
	if os.Getenv("HONK_URL") == "" || os.Getenv("HONK_KEY") == "" {
		t.Skip("set HONK_URL and HONK_KEY to run against a real server")
	}
	env := map[string]string{"HONK_URL": os.Getenv("HONK_URL"), "HONK_KEY": os.Getenv("HONK_KEY"), "HONK_SOURCE": "sdk-cli-it"}
	run := fmt.Sprintf("cli-%d", time.Now().UnixNano())
	key := "it-" + run

	code, out, errOut := runCLI(t, env, "", "send", "--title", "CLI "+run, "--message", "from the honk CLI", "--severity", "success",
		"--category", "automation", "--meta", "run="+run, "--meta", "n:=3", "--image-url", "https://example.com/i.png",
		"--action", "Open run=https://example.com/runs/"+run, "--action", "Call on-call=tel:+15550134",
		"--idempotency-key", key, "--json")
	if code != exitOK || !strings.Contains(out, `"duplicate":false`) || !strings.Contains(out, `"id":"msg_`) {
		t.Fatalf("send: %d %q %q", code, out, errOut)
	}
	code, out, _ = runCLI(t, env, "", "send", "--title", "CLI "+run, "--message", "from the honk CLI", "--severity", "success",
		"--category", "automation", "--meta", "run="+run, "--meta", "n:=3", "--image-url", "https://example.com/i.png",
		"--action", "Open run=https://example.com/runs/"+run, "--action", "Call on-call=tel:+15550134",
		"--idempotency-key", key, "--json")
	if code != exitOK || !strings.Contains(out, `"duplicate":true`) {
		t.Fatalf("replay: %d %q", code, out)
	}
	if code, _, e := runCLI(t, env, "", "send", "--message", "different", "--idempotency-key", key); code != exitConflict {
		t.Fatalf("conflict: %d %s", code, e)
	}
	if code, _, e := runCLI(t, env, "", "uhoh", "--group-key", "it/cli/"+run, "--title", "Job failed", "--message", "exit 1", "--idempotency-key", key+"-p", "--quiet"); code != exitOK {
		t.Fatalf("uhoh: %d %s", code, e)
	}
	if code, _, e := runCLI(t, env, "", "phew", "--group-key", "it/cli/"+run, "--title", "Job OK", "--message", "exit 0", "--idempotency-key", key+"-r", "--quiet"); code != exitOK {
		t.Fatalf("phew: %d %s", code, e)
	}
	// The old subcommands send the very same requests: the server sees replays.
	if code, out, e := runCLI(t, env, "", "problem", "--group-key", "it/cli/"+run, "--title", "Job failed", "--message", "exit 1", "--idempotency-key", key+"-p", "--json"); code != exitOK || !strings.Contains(out, `"duplicate":true`) {
		t.Fatalf("problem: %d %q %s", code, out, e)
	}
	if code, out, e := runCLI(t, env, "", "recovery", "--group-key", "it/cli/"+run, "--title", "Job OK", "--message", "exit 0", "--idempotency-key", key+"-r", "--json"); code != exitOK || !strings.Contains(out, `"duplicate":true`) {
		t.Fatalf("recovery: %d %q %s", code, out, e)
	}
	// GROUP_KEY [TITLE] MESSAGE as arguments: the same request as the flags (a replay).
	pos := "it/cli/" + run + "/pos"
	if code, _, e := runCLI(t, env, "", "uhoh", pos, "Backup failed", "Exit code 1", "--idempotency-key", key+"-pos", "--quiet"); code != exitOK {
		t.Fatalf("uhoh GROUP_KEY TITLE MESSAGE: %d %s", code, e)
	}
	if code, out, e := runCLI(t, env, "", "uhoh", "--group-key", pos, "--title", "Backup failed", "--message", "Exit code 1", "--idempotency-key", key+"-pos", "--json"); code != exitOK || !strings.Contains(out, `"duplicate":true`) {
		t.Fatalf("uhoh flags after arguments: %d %q %s", code, out, e)
	}
	if code, _, e := runCLI(t, env, "", "phew", pos, "Backup is back", "All good", "--quiet"); code != exitOK {
		t.Fatalf("phew GROUP_KEY TITLE MESSAGE: %d %s", code, e)
	}
	if code, _, e := runCLI(t, env, "", "loud", "Disk 91%", "/var on cli-it "+run, "--quiet"); code != exitOK {
		t.Fatalf("loud shortcut: %d %s", code, e)
	}
	bad := map[string]string{"HONK_URL": env["HONK_URL"], "HONK_KEY": "honk_000000000000_00000000000000000000000000000000"}
	if code, _, e := runCLI(t, bad, "", "send", "--message", "x"); code != exitAuth {
		t.Fatalf("wrong key: %d %s", code, e)
	}
	if code, _, e := runCLI(t, env, "", "send", "--message", "x", "--priority", "urgent"); code != exitAuth && code != exitOK {
		t.Fatalf("urgent: %d %s", code, e)
	}
}
