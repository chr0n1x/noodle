package loop

import (
	"fmt"
	"strings"
)

// skillWordingForProvider rewrites a prompt's "Use Skill(<skill>)" lines for
// providers whose dispatcher inlines the skill into the system prompt
// (claude). Those agents cannot load .agents/skills through the Skill tool,
// and calling it can load a different skill of the same name from the user's
// own Claude Code setup.
func skillWordingForProvider(prompt, provider, skill string) string {
	skill = strings.TrimSpace(skill)
	if skill == "" || !strings.EqualFold(strings.TrimSpace(provider), "claude") {
		return prompt
	}
	prompt = strings.Replace(prompt,
		fmt.Sprintf("Use Skill(%s) for methodology.", skill),
		fmt.Sprintf("Follow the %s skill in your system prompt for methodology. It is already loaded; do not call the Skill tool for it.", skill),
		1)
	return strings.ReplaceAll(prompt,
		fmt.Sprintf("Use Skill(%s)", skill),
		fmt.Sprintf("Follow the %s skill in your system prompt (it is already loaded; do not call the Skill tool)", skill))
}
