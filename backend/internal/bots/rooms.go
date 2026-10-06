package bots

import (
	"context"
	"errors"
	"fmt"
	"slices"
	"strings"
	"sync"
	"time"

	"github.com/wins/jaz/backend/internal/acp"
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

// memberTurns schedules a bot's turns in one group: reading lets one caller at
// a time show the bot group messages, so none is shown twice; taking and owed,
// guarded by Service.mu, say whether a turn is in flight and whether another is
// owed after it.
type memberTurns struct {
	reading      sync.Mutex
	taking, owed bool
}

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
// hear it. A member taking a turn in the group hears every post during that
// turn; otherwise the message wakes the members it addresses, each in a turn
// of its own, all at once.
func (s *Service) post(group storage.BotRecord, message sessionevents.RoomMessageEvent) error {
	text, mentioned := s.resolveMentions(message.Text, group.Members)
	message.Text = text
	if err := s.appendEvent(sessionevents.Event{SessionID: group.ThreadID, Type: sessionevents.TypeRoomMessage, RoomMessage: &message, At: time.Now().UTC()}); err != nil {
		return err
	}
	fromMember := slices.Contains(group.Members, message.BotID)
	wake := addressed(group.Members, mentioned, fromMember)
	s.mu.Lock()
	defer s.mu.Unlock()
	if !fromMember {
		s.followUps[group.ThreadID] = 0
	}
	for _, member := range group.Members {
		if member == message.BotID {
			continue
		}
		turns := s.memberTurnsLocked(group.ThreadID, member)
		if !turns.taking && !slices.Contains(wake, member) {
			continue
		}
		if fromMember {
			if s.followUps[group.ThreadID] >= maxFollowUps {
				break
			}
			s.followUps[group.ThreadID]++
		}
		if turns.taking {
			go s.relay(group.ThreadID, member, turns)
		} else {
			s.wakeLocked(group.ThreadID, member)
		}
	}
	return nil
}

// addressed is who a group message wakes: the members it mentions or, with no
// mention, every member when the user or an outsider wrote it and nobody when
// a member did, since bots follow up on each other only when addressed.
func addressed(members, mentioned []string, fromMember bool) []string {
	switch {
	case mentioned != nil:
		return mentioned
	case fromMember:
		return nil
	}
	return members
}

// ResumeGroups wakes, after a restart, every member with messages addressed to
// it in a group that it has not been shown.
func (s *Service) ResumeGroups() error {
	memberships, err := s.store.ListMemberships()
	if err != nil {
		return err
	}
	for _, membership := range memberships {
		group, _, err := s.load(membership.GroupID)
		if err != nil || !slices.Contains(group.Members, membership.BotID) {
			continue
		}
		messages, _, err := s.unseen(membership.GroupID, membership.BotID, membership.Seen)
		if err != nil {
			return err
		}
		if !slices.ContainsFunc(messages, func(message sessionevents.RoomMessageEvent) bool {
			_, mentioned := s.resolveMentions(message.Text, group.Members)
			return slices.Contains(addressed(group.Members, mentioned, slices.Contains(group.Members, message.BotID)), membership.BotID)
		}) {
			continue
		}
		s.mu.Lock()
		s.wakeLocked(membership.GroupID, membership.BotID)
		s.mu.Unlock()
	}
	return nil
}

// memberTurnsLocked returns how member's turns in the group are scheduled.
// Callers hold s.mu.
func (s *Service) memberTurnsLocked(groupID, member string) *memberTurns {
	key := groupID + "\x00" + member
	turns := s.members[key]
	if turns == nil {
		turns = &memberTurns{}
		s.members[key] = turns
	}
	return turns
}

// wakeLocked gives member a turn in the group soon. A member has at most one
// turn in flight per group: a wake while it is busy there folds into one more
// turn, which answers whatever that member has not seen by then, so a burst of
// posts costs one turn. Callers hold s.mu.
func (s *Service) wakeLocked(groupID, member string) {
	turns := s.memberTurnsLocked(groupID, member)
	if turns.taking {
		turns.owed = true
		return
	}
	turns.taking = true
	go s.takeTurns(groupID, member, turns)
}

// takeTurns runs member's turns in the group until no wake is owed.
func (s *Service) takeTurns(groupID, member string, turns *memberTurns) {
	for {
		s.mu.Lock()
		turns.owed = false
		s.mu.Unlock()
		if err := s.memberTurn(groupID, member, turns); err != nil {
			s.log.Warn("group turn failed", "group", groupID, "member", member, "error", err)
		}
		s.mu.Lock()
		owed := turns.owed
		turns.taking = owed
		s.mu.Unlock()
		if !owed {
			return
		}
	}
}

// memberTurn gives member a turn in its thread for the group, to answer the
// messages it has not seen there, posting with send_message. With none left,
// it takes no turn.
func (s *Service) memberTurn(groupID, member string, turns *memberTurns) error {
	thread, job, err := s.beginMemberTurn(groupID, member, turns)
	if err != nil || thread == "" {
		return err
	}
	done, err := s.finish(context.Background(), thread, job.ID)
	if err != nil {
		return err
	}
	if done.State == acp.StateFailed {
		return fmt.Errorf("%s failed: %s", thread, done.Error)
	}
	return nil
}

// beginMemberTurn starts member's turn in its thread for the group with the
// messages it has not seen, and marks them seen once the turn has started. It
// reads for the member until then, so posts that arrive while the turn waits
// to start reach it as soon as it runs. It returns no thread when there is
// nothing to answer.
func (s *Service) beginMemberTurn(groupID, member string, turns *memberTurns) (string, acp.Job, error) {
	group, _, err := s.load(groupID)
	if err != nil {
		return "", acp.Job{}, err
	}
	turns.reading.Lock()
	defer turns.reading.Unlock()
	membership, err := s.membership(groupID, member)
	if err != nil {
		return "", acp.Job{}, err
	}
	messages, seen, err := s.unseen(groupID, member, membership.Seen)
	if err != nil || len(messages) == 0 {
		return "", acp.Job{}, err
	}
	peers := make([]string, 0, len(group.Members))
	for _, other := range group.Members {
		if other != member {
			peers = append(peers, fmt.Sprintf("[@%s](bot:%s)", s.name(other), other))
		}
	}
	job, err := s.threads.StartInternalTurnWhenIdle(context.Background(), acp.InternalTurnRequest{
		Session: membership.ThreadID, Message: groupTurnPrompt(peers, messages), AllowSilence: true,
	})
	if err != nil {
		return "", acp.Job{}, err
	}
	membership.Seen = seen
	return membership.ThreadID, job, s.store.SaveMembership(membership)
}

// membership returns member's place in the group, creating the thread it takes
// the group's turns in before its first turn there: a hidden thread on the
// bot's agent, model and home.
func (s *Service) membership(groupID, member string) (storage.BotMembership, error) {
	membership, err := s.store.LoadMembership(groupID, member)
	if !errors.Is(err, storage.ErrMembershipNotFound) {
		return membership, err
	}
	bot, err := s.store.LoadSession(member)
	if err != nil {
		return storage.BotMembership{}, err
	}
	group, err := s.store.LoadSession(groupID)
	if err != nil {
		return storage.BotMembership{}, err
	}
	request := acp.SpawnRequest{
		Slug:            bot.Title + " in " + group.Title,
		Title:           bot.Title + " in " + group.Title,
		ModelProvider:   bot.ModelProvider,
		Model:           bot.Model,
		ReasoningEffort: bot.ReasoningEffort,
		SourceType:      storage.SourceBotMember,
		SourceID:        member,
	}
	if ref := bot.RuntimeRef; ref != nil {
		request.ACPAgent, request.Directory = ref.Agent, ref.Cwd
	}
	thread, err := s.threads.CreateSession(context.Background(), request)
	if err != nil {
		return storage.BotMembership{}, err
	}
	membership = storage.BotMembership{GroupID: groupID, BotID: member, ThreadID: thread.ID}
	return membership, s.store.SaveMembership(membership)
}

// relay hands member the group messages it has not seen while it works on its
// turn there, so it can work them in before the turn ends. A turn that has
// ended or cannot take them owes the member another turn instead.
func (s *Service) relay(groupID, member string, turns *memberTurns) {
	turns.reading.Lock()
	defer turns.reading.Unlock()
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
// the member's reading.
func (s *Service) steerUnseen(groupID, member string) error {
	membership, err := s.store.LoadMembership(groupID, member)
	if err != nil {
		return err
	}
	messages, seen, err := s.unseen(groupID, member, membership.Seen)
	if err != nil || len(messages) == 0 {
		return err
	}
	thread, err := s.store.LoadSession(membership.ThreadID)
	if err != nil {
		return err
	}
	if thread.Turn == nil {
		return errors.New("its turn in the group has ended")
	}
	ctx, cancel := context.WithTimeout(context.Background(), relayTimeout)
	defer cancel()
	if _, err := s.threads.SteerInternal(ctx, membership.ThreadID, groupUpdatePrompt(messages)); err != nil {
		return err
	}
	membership.Seen = seen
	return s.store.SaveMembership(membership)
}

// unseen returns the group messages after seq after that member did not post,
// with the seq that marks them seen. With after zero, as before member has
// been shown anything in its group thread, it returns those after member's own
// last post.
func (s *Service) unseen(groupID, member string, after int64) ([]sessionevents.RoomMessageEvent, int64, error) {
	events, err := s.store.LoadSessionEvents(groupID)
	if err != nil {
		return nil, 0, err
	}
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
