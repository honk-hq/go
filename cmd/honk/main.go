// Command honk sends one event to Honk from a shell script, cron job or CI step.
//
//	honk loud "Disk 91%"                                   # shortcut per horn: light beep loud long blast
//	honk send --title "Backup finished" --message "42 s" --severity beep
//	honk uhoh --group-key db/backup --title "Backup failed" --message "exit 1"
//	honk phew --group-key db/backup --title "Backup OK" --message "fixed"
//
// HONK_URL and HONK_KEY come from the environment. Run `honk help` for flags and exit codes.
//
//	go install github.com/honk-me/honk-go/cmd/honk@latest
package main

import "github.com/honk-me/honk-go/internal/cli"

func main() { cli.Main() }
