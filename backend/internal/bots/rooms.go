package bots

import (
	"context"
	"errors"
	"fmt"
	"slices"
	"strings"
	"time"

	"github.com/wins/jaz/backend/internal/sessionevents"
	"github.com/wins/jaz/backend/internal/storage"
)

const (
	// maxFollowUps bounds the turns bots wake in each other between two posts
	// from the user, so a mention ping-pong cannot run on.
	maxFollowUps = 6
	maxHistory   = 20
)

// Post adds the user's message to a group.
func (s *Service) Post(groupID, text string) error {
	text = strings.TrimSpace(text)
	if text == "" {
		return errors.New("message is required")
	}
	record, session, err := s.load(groupID)
	if err != nil {
		return err
	}
	if record.Kind != KindGroup {
		return errors.New("not a group")
	}
	return s.post(record, session.Title, sessionevents.RoomMessageEvent{Speaker: "user", Name: "You", Text: text})
}

// post records a message in a group and wakes the members it mentions, each in
// a turn of its own, all at once. A message that mentions nobody wakes every
// member when the user or an outsider wrote it, and nobody when a member did:
// bots follow up on each other only when addressed.
func (s *Service) post(group storage.BotRecord, name string, message sessionevents.RoomMessageEvent) error {
	text, mentioned := s.resolveMentions(message.Text, group.Members)
	message.Text = text
	if err := s.appendEvent(sessionevents.Event{SessionID: group.ThreadID, Type: sessionevents.TypeRoomMessage, RoomMessage: &message, At: time.Now().UTC()}); err != nil {
		return err
	}
	fromMember := slices.Contains(group.Members, message.BotID)
	wake := group.Members
	if mentioned != nil {
		wake = mentioned
	} else if fromMember {
		wake = nil
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if !fromMember {
		s.followUps[group.ThreadID] = 0
	}
	for _, member := range wake {
		if member == message.BotID || !slices.Contains(group.Members, member) {
			continue
		}
		if fromMember {
			if s.followUps[group.ThreadID] >= maxFollowUps {
				break
			}
			s.followUps[group.ThreadID]++
		}
		s.wakeLocked(group.ThreadID, name, member)
	}
	return nil
}

// wakeLocked gives member a turn in the group soon. A member has at most one
// turn in flight per group: a wake while it is busy there folds into one more
// turn, whose prompt is built as that turn begins, so a burst of posts costs
// one turn. Callers hold s.mu.
func (s *Service) wakeLocked(groupID, name, member string) {
	key := groupID + "\x00" + member
	if _, busy := s.waking[key]; busy {
		s.waking[key] = true
		return
	}
	s.waking[key] = false
	go s.takeTurns(key, groupID, name, member)
}

// takeTurns runs member's turns in the group until no wake is owed.
func (s *Service) takeTurns(key, groupID, name, member string) {
	for {
		s.mu.Lock()
		s.waking[key] = false
		s.mu.Unlock()
		if err := s.memberTurn(groupID, name, member); err != nil {
			s.log.Warn("group turn failed", "group", groupID, "member", member, "error", err)
		}
		s.mu.Lock()
		owed := s.waking[key]
		if !owed {
			delete(s.waking, key)
		}
		s.mu.Unlock()
		if !owed {
			return
		}
	}
}

// memberTurn gives member a turn in the group, in which it posts with
// send_message.
func (s *Service) memberTurn(groupID, name, member string) error {
	record, err := s.store.LoadBot(groupID)
	if err != nil {
		return err
	}
	events, err := s.store.LoadSessionEvents(groupID)
	if err != nil {
		return err
	}
	peers := make([]string, 0, len(record.Members))
	for _, other := range record.Members {
		if other != member {
			peers = append(peers, fmt.Sprintf("[@%s](bot:%s)", s.name(other), other))
		}
	}
	prompt := groupTurnPrompt(name, s.name(member), peers, unseen(events, member))
	_, err = s.ask(context.Background(), member, groupID, prompt, sessionevents.BotActivityEvent{Kind: "group", Label: name})
	return err
}

// unseen returns the group messages posted since member last spoke.
func unseen(events []sessionevents.Event, member string) []sessionevents.RoomMessageEvent {
	var messages []sessionevents.RoomMessageEvent
	for _, event := range events {
		message := event.RoomMessage
		if message == nil {
			continue
		}
		if message.BotID == member {
			messages = messages[:0]
			continue
		}
		messages = append(messages, *message)
	}
	if len(messages) > maxHistory {
		messages = messages[len(messages)-maxHistory:]
	}
	return messages
}
