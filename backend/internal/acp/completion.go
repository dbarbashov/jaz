package acp

import (
	"fmt"

	"github.com/wins/jaz/backend/internal/templates/acpcompletion"
)

func CompletionPrompt(job Job) string {
	prompt, err := acpcompletion.Render(acpcompletion.Data{
		Slug: job.Slug, Agent: job.ACPAgent, State: job.State, Error: job.Error, Assistant: job.Assistant,
	})
	if err != nil {
		return fmt.Sprintf("ACP session %s (%s) completed with state %s. Continue from this result and report/update the user with relevant details.", job.Slug, job.ACPAgent, job.State)
	}
	return prompt
}
