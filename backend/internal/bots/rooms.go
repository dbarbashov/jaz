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

// membership is a bot's place in a group.
type membership struct {
	// reading lets one caller at a time show the bot group messages, so none
	// is shown twice. It guards seen, the seq of the last group message shown
	// to the bot.
	reading sync.Mutex
	seen    int64
	// taking and owed, guarded by Service.mu, say whether the bot has a turn
	// in flight in the group and whether another is owed after it.
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
		if member == message.BotID {
			continue
		}
		place := s.membershipLocked(group.ThreadID, member)
		if !place.taking && !slices.Contains(wake, member) {
			continue
		}
		if fromMember {
			if s.followUps[group.ThreadID] >= maxFollowUps {
				break
			}
			s.followUps[group.ThreadID]++
		}
		if place.taking {
			go s.relay(group.ThreadID, member, place)
		} else {
			s.wakeLocked(group.ThreadID, member)
		}
	}
	return nil
}

// membershipLocked returns member's place in the group. Callers hold s.mu.
func (s *Service) membershipLocked(groupID, member string) *membership {
	key := groupID + "\x00" + member
	place := s.members[key]
	if place == nil {
		place = &membership{}
		s.members[key] = place
	}
	return place
}

// wakeLocked gives member a turn in the group soon. A member has at most one
// turn in flight per group: a wake while it is busy there folds into one more
// turn, which answers whatever that member has not seen by then, so a burst of
// posts costs one turn. Callers hold s.mu.
func (s *Service) wakeLocked(groupID, member string) {
	place := s.membershipLocked(groupID, member)
	if place.taking {
		place.owed = true
		return
	}
	place.taking = true
	go s.takeTurns(groupID, member, place)
}

// takeTurns runs member's turns in the group until no wake is owed.
func (s *Service) takeTurns(groupID, member string, place *membership) {
	for {
		s.mu.Lock()
		place.owed = false
		s.mu.Unlock()
		if err := s.memberTurn(groupID, member, place); err != nil {
			s.log.Warn("group turn failed", "group", groupID, "member", member, "error", err)
		}
		s.mu.Lock()
		owed := place.owed
		place.taking = owed
		s.mu.Unlock()
		if !owed {
			return
		}
	}
}

// memberTurn gives member a turn in the group to answer the messages it has
// not seen, posting with send_message. With none left, it takes no turn.
func (s *Service) memberTurn(groupID, member string, place *membership) error {
	turn, job, err := s.beginMemberTurn(groupID, member, place)
	if err != nil || turn == nil {
		return err
	}
	_, err = s.hear(context.Background(), member, turn, job)
	return err
}

// beginMemberTurn starts member's turn with the messages it has not seen and
// marks them seen once the turn has started. It reads for the member until
// then, so posts that arrive while the turn waits to start reach it as soon as
// it runs.
func (s *Service) beginMemberTurn(groupID, member string, place *membership) (*voice, acp.Job, error) {
	record, session, err := s.load(groupID)
	if err != nil {
		return nil, acp.Job{}, err
	}
	place.reading.Lock()
	defer place.reading.Unlock()
	messages, seen, err := s.unseen(groupID, member, place.seen)
	if err != nil || len(messages) == 0 {
		return nil, acp.Job{}, err
	}
	peers := make([]string, 0, len(record.Members))
	for _, other := range record.Members {
		if other != member {
			peers = append(peers, fmt.Sprintf("[@%s](bot:%s)", s.name(other), other))
		}
	}
	prompt := groupTurnPrompt(session.Title, s.name(member), peers, messages)
	turn, job, err := s.begin(context.Background(), member, groupID, prompt, sessionevents.BotActivityEvent{Kind: "group", Label: session.Title})
	if err == nil {
		place.seen = seen
	}
	return turn, job, err
}

// relay hands member the group messages it has not seen while it works on its
// turn there, so it can work them in before the turn ends. A turn that has
// ended or cannot take them owes the member another turn instead.
func (s *Service) relay(groupID, member string, place *membership) {
	place.reading.Lock()
	defer place.reading.Unlock()
	err := s.steerUnseen(groupID, member, place)
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
// place.reading.
func (s *Service) steerUnseen(groupID, member string, place *membership) error {
	_, session, err := s.load(groupID)
	if err != nil {
		return err
	}
	messages, seen, err := s.unseen(groupID, member, place.seen)
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
	place.seen = seen
	return nil
}

// unseen returns the group messages after seq after that member did not post,
// with the seq that marks them seen. With after zero, as when Jaz has shown
// member nothing since starting, it returns those after member's own last post.
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
