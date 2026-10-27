package loop

import (
	"fmt"
	"strings"
)

// skillWordingForProvider rewrites the cook prompt's skill line for providers
// whose dispatcher inlines the skill into the system prompt (claude). Those
// agents cannot load .agents/skills through the Skill tool.
func skillWordingForProvider(prompt, provider, skill string) string {
	skill = strings.TrimSpace(skill)
	if skill == "" || !strings.EqualFold(strings.TrimSpace(provider), "claude") {
		return prompt
	}
	return strings.Replace(prompt,
		fmt.Sprintf("Use Skill(%s) for methodology.", skill),
		fmt.Sprintf("Follow the %s skill in your system prompt for methodology. It is already loaded; do not call the Skill tool for it.", skill),
		1)
}
