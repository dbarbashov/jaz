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
	// maxFollowUps bounds how often members' posts reach each other between two
	// posts from the user, as a new turn or mid-turn, so bots answering each
	// other cannot run on.
	maxFollowUps = 12
	maxHistory   = 20
	// relayTimeout bounds how long a relay waits for a turn to take its
	// messages.
	relayTimeout = time.Minute
)

// Post adds the user's message to a group.
func (s *Service) Post(groupID, text string) error {
	text = strings.TrimSpace(text)
	if text == "" {
		return errors.New("message is required")
	}
	record, _, err := s.load(groupID)
	if err != nil {
		return err
	}
	if record.Kind != KindGroup {
		return errors.New("not a group")
	}
	return s.post(record, sessionevents.RoomMessageEvent{Speaker: "user", Name: "You", Text: text})
}

// post records a message in a group and hands it to the members who should
// hear it. A member working on its turn in the group hears every post during
// that turn. Otherwise the members a message mentions wake, each in a turn of
// its own, all at once. A message that mentions nobody wakes every member when
// the user or an outsider wrote it, and nobody when a member did: bots follow
// up on each other only when addressed. A member whose turn has not started
// yet gets the posts that arrive meanwhile as soon as it starts.
func (s *Service) post(group storage.BotRecord, message sessionevents.RoomMessageEvent) error {
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
	for _, member := range group.Members {
		working := s.voices[member].in(group.ThreadID)
		if member == message.BotID || !working && !slices.Contains(wake, member) {
			continue
		}
		if fromMember {
			if s.followUps[group.ThreadID] >= maxFollowUps {
				break
			}
			s.followUps[group.ThreadID]++
		}
		if working {
			go s.relay(group.ThreadID, member)
		} else {
			s.wakeLocked(group.ThreadID, member)
		}
	}
	return nil
}

func turnKey(groupID, member string) string {
	return groupID + "\x00" + member
}

// wakeLocked gives member a turn in the group soon. A member has at most one
// turn in flight per group: a wake while it is busy there folds into one more
// turn, which answers whatever that member has not seen by then, so a burst of
// posts costs one turn. Callers hold s.mu.
func (s *Service) wakeLocked(groupID, member string) {
	key := turnKey(groupID, member)
	if _, busy := s.waking[key]; busy {
		s.waking[key] = true
		return
	}
	s.waking[key] = false
	go s.takeTurns(key, groupID, member)
}

// takeTurns runs member's turns in the group until no wake is owed.
func (s *Service) takeTurns(key, groupID, member string) {
	for {
		s.mu.Lock()
		s.waking[key] = false
		s.mu.Unlock()
		if err := s.memberTurn(groupID, member); err != nil {
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

// memberTurn gives member a turn in the group to answer the messages it has
// not seen, posting with send_message. With none left, it takes no turn.
func (s *Service) memberTurn(groupID, member string) error {
	record, session, err := s.load(groupID)
	if err != nil {
		return err
	}
	s.relaying.Lock()
	messages, seen, err := s.unseen(groupID, member)
	if err == nil {
		s.markSeen(groupID, member, seen)
	}
	s.relaying.Unlock()
	if err != nil || len(messages) == 0 {
		return err
	}
	peers := make([]string, 0, len(record.Members))
	for _, other := range record.Members {
		if other != member {
			peers = append(peers, fmt.Sprintf("[@%s](bot:%s)", s.name(other), other))
		}
	}
	prompt := groupTurnPrompt(session.Title, s.name(member), peers, messages)
	_, err = s.ask(context.Background(), member, groupID, prompt, sessionevents.BotActivityEvent{Kind: "group", Label: session.Title})
	return err
}

// relay hands member the group's messages it has not seen while it works on
// its turn there, so it can work them in before the turn ends. A turn that has
// ended or cannot take them owes the member another turn instead.
func (s *Service) relay(groupID, member string) {
	s.relaying.Lock()
	defer s.relaying.Unlock()
	err := s.steerUnseen(groupID, member)
	if err == nil {
		return
	}
	s.log.Debug("group relay fell back to a turn", "group", groupID, "member", member, "error", err)
	s.mu.Lock()
	s.wakeLocked(groupID, member)
	s.mu.Unlock()
}

// steerUnseen hands the messages member has not seen, if any, to its running
// turn in the group and marks them seen once the turn has them. Callers hold
// s.relaying.
func (s *Service) steerUnseen(groupID, member string) error {
	_, session, err := s.load(groupID)
	if err != nil {
		return err
	}
	messages, seen, err := s.unseen(groupID, member)
	if err != nil || len(messages) == 0 {
		return err
	}
	s.mu.Lock()
	working := s.voices[member].in(groupID)
	s.mu.Unlock()
	if !working {
		return errors.New("its turn in the group has ended")
	}
	ctx, cancel := context.WithTimeout(context.Background(), relayTimeout)
	defer cancel()
	if _, err := s.threads.SteerInternal(ctx, member, groupUpdatePrompt(session.Title, messages)); err != nil {
		return err
	}
	s.markSeen(groupID, member, seen)
	return nil
}

func (s *Service) markSeen(groupID, member string, seq int64) {
	s.mu.Lock()
	s.seen[turnKey(groupID, member)] = seq
	s.mu.Unlock()
}

// unseen returns the group messages member has not been shown, with the seq
// that marks them seen: those after the last message shown to it or, when Jaz
// has shown it none since starting, after its own last post.
func (s *Service) unseen(groupID, member string) ([]sessionevents.RoomMessageEvent, int64, error) {
	events, err := s.store.LoadSessionEvents(groupID)
	if err != nil {
		return nil, 0, err
	}
	s.mu.Lock()
	after := s.seen[turnKey(groupID, member)]
	s.mu.Unlock()
	seen := after
	var messages []sessionevents.RoomMessageEvent
	for _, event := range events {
		message := event.RoomMessage
		if message == nil || event.Seq <= after {
			continue
		}
		seen = event.Seq
		switch {
		case message.BotID != member:
			messages = append(messages, *message)
		case after == 0:
			messages = messages[:0]
		}
	}
	if len(messages) > maxHistory {
		messages = messages[len(messages)-maxHistory:]
	}
	return messages, seen, nil
}
