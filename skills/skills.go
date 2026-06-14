// Package skills embeds the Claude Code skills that `nitpick install` writes
// into ~/.claude/skills, so a `go install`-ed binary carries them with no
// separate download.
package skills

import "embed"

// FS holds every embedded skill directory, rooted at the skill name.
//
//go:embed nitpick
var FS embed.FS
