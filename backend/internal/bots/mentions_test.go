package bots

import (
	"slices"
	"testing"

	"github.com/wins/jaz/backend/internal/sessionevents"
)

func TestGroupMentionRouting(t *testing.T) {
	for _, tc := range []struct {
		name string
		text string
		want []string
	}{
		{"named", "[@Business Opportunist] Please check this.", []string{"b"}},
		{"linked", "[@Business Opportunist](bot:b) Please check this.", []string{"b"}},
		{"duplicate", "[@Business Opportunist] [@Business Opportunist](bot:b)", []string{"b"}},
		{"explicit ID wins", "[@Business Opportunist](bot:c)", []string{"c"}},
		{"reference link", "[@Business Opportunist][peer]\n\n[peer]: bot:b", []string{"b"}},
		{"unknown", "[@Missing]", nil},
		{"ambiguous", "[@Shared name]", nil},
		{"ambiguous with ID", "[@Shared name](bot:d)", []string{"d"}},
		{"outsider", "[@Outside](bot:outside)", nil},
		{"self", "[@Researcher]", nil},
		{"ordinary prose", "Business Opportunist has an idea.", nil},
		{"code", "`[@Business Opportunist]` and `[@Business Opportunist](bot:b)`", nil},
		{"fenced code", "```text\n[@Business Opportunist](bot:b)\n```", nil},
		{"web link", "[@Business Opportunist](https://example.com)", nil},
		{"image", "![@Business Opportunist](bot:b)", nil},
	} {
		t.Run(tc.name, func(t *testing.T) {
			world := newFakeWorld()
			world.addBot("a", "Researcher")
			world.addBot("b", "Business Opportunist")
			world.addBot("c", "Planner")
			world.addBot("d", "Shared name")
			world.addBot("e", "Shared name")
			service := newTestService(world)
			group, err := service.CreateGroup("Discovery", []string{"a", "b", "c", "d", "e"})
			if err != nil {
				t.Fatal(err)
			}
			record, err := world.LoadBot(group.ID)
			if err != nil {
				t.Fatal(err)
			}
			err = service.post(record, group.Name, sessionevents.RoomMessageEvent{
				Speaker: "bot", BotID: "a", Name: "Researcher", Text: tc.text,
			})
			if err != nil {
				t.Fatal(err)
			}
			waitUntil(t, func() bool {
				service.mu.Lock()
				defer service.mu.Unlock()
				return len(service.waking) == 0
			})
			for _, id := range record.Members {
				want := 0
				if slices.Contains(tc.want, id) {
					want = 1
				}
				if got := world.promptCount(id); got != want {
					t.Errorf("bot %s got %d turns, want %d", id, got, want)
				}
			}
		})
	}
}
