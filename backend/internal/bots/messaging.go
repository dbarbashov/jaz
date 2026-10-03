package bots

import (
	"context"
	"errors"
	"fmt"
	"slices"
	"strings"
	"time"

	"github.com/wins/jaz/backend/internal/acp"
	"github.com/wins/jaz/backend/internal/sessionevents"
	"github.com/wins/jaz/backend/internal/storage"
)

// turnTimeout bounds how long a turn this service starts keeps its voice. No
// real turn runs this long; ending the voice early would send the rest of a
// still-running turn to the wrong chat.
const turnTimeout = 12 * time.Hour

// voice is where a bot's messages go during a turn this service started for
// it: into group, or, with no group, back to the bot that wrote to it once the
// turn ends. Outside such a turn a bot talks in its own chat.
type voice struct {
	group string
	said  []string
}

func (s *Service) AppVisible(threadID string) bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.voices[threadID] == nil
}

// Message sends text from a thread to a bot or a group, named by id, mention
// target or name. A bot answers in a turn of its own and the answer returns to
// the sender as a new turn; a group gets the text as a post and its members
// take their turns.
func (s *Service) Message(fromThread, ref, text string) error {
	text = strings.TrimSpace(text)
	if text == "" {
		return errors.New("message is required")
	}
	to, err := s.resolve(ref)
	if err != nil {
		return err
	}
	target, session, err := s.load(to)
	if err != nil {
		return err
	}
	sender := s.name(fromThread)
	if target.Kind == KindGroup {
		speaker := sessionevents.RoomMessageEvent{Speaker: "bot", BotID: fromThread, Name: sender, Text: text}
		return s.post(target, session.Title, speaker)
	}
	if to == fromThread {
		return errors.New("a bot cannot message itself")
	}
	s.announce(fromThread, sessionevents.BotActivityEvent{Kind: "message_sent", Label: session.Title})
	go s.deliver(fromThread, sender, to, session.Title, text)
	return nil
}

// Say posts a bot's message: into the group whose turn it is taking, to the
// bot it is answering, or else into its own chat.
func (s *Service) Say(threadID, text string) error {
	text = strings.TrimSpace(text)
	if text == "" {
		return errors.New("message is required")
	}
	record, session, err := s.load(threadID)
	if err != nil || record.Kind != KindBot {
		return errors.New("only a Jaz bot can send messages")
	}
	s.mu.Lock()
	turn := s.voices[threadID]
	if turn != nil {
		turn.said = append(turn.said, text)
	}
	s.mu.Unlock()
	message := sessionevents.RoomMessageEvent{Speaker: "bot", BotID: threadID, Name: session.Title, Text: text}
	switch {
	case turn == nil:
		return s.appendEvent(sessionevents.Event{SessionID: threadID, Type: sessionevents.TypeRoomMessage, RoomMessage: &message, At: time.Now().UTC()})
	case turn.group == "":
		return nil
	}
	group, groupSession, err := s.load(turn.group)
	if err != nil {
		return err
	}
	return s.post(group, groupSession.Title, message)
}

func (s *Service) deliver(fromThread, sender, to, recipient, text string) {
	ctx, cancel := context.WithTimeout(context.Background(), turnTimeout)
	defer cancel()
	said, err := s.ask(ctx, to, "", messagePrompt(sender, text), sessionevents.BotActivityEvent{Kind: "message_received", Label: sender})
	if err != nil {
		s.log.Warn("bot message failed", "from", fromThread, "to", to, "error", err)
		return
	}
	if fromThread == "" || len(said) == 0 {
		return
	}
	if _, err := s.threads.StartInternalTurnWhenIdle(ctx, acp.InternalTurnRequest{Session: fromThread, Message: replyPrompt(recipient, strings.Join(said, "\n\n")), AllowSilence: true}); err != nil {
		s.log.Warn("bot reply delivery failed", "from", to, "to", fromThread, "error", err)
		return
	}
	s.announce(fromThread, sessionevents.BotActivityEvent{Kind: "message_received", Label: recipient})
}

// RunRoutine runs a routine's prompt as a hidden turn in its bot's thread once
// the thread is free, and returns that turn once it ends.
func (s *Service) RunRoutine(ctx context.Context, botID, name, prompt string) (acp.Job, error) {
	job, err := s.threads.StartInternalTurnWhenIdle(ctx, acp.InternalTurnRequest{Session: botID, Message: routinePrompt(prompt), AllowSilence: true})
	if err != nil {
		return acp.Job{}, err
	}
	s.announce(botID, sessionevents.BotActivityEvent{Kind: "routine", Label: name})
	return s.finish(ctx, botID, job.ID)
}

// ask runs one hidden turn in a bot's thread once it is free and returns what
// the bot said in it. The turn keeps its voice until it ends, even when ctx is
// cancelled first.
func (s *Service) ask(ctx context.Context, threadID, group, prompt string, activity sessionevents.BotActivityEvent) ([]string, error) {
	job, err := s.threads.StartInternalTurnWhenIdle(ctx, acp.InternalTurnRequest{Session: threadID, Message: prompt, AllowSilence: true})
	if err != nil {
		return nil, err
	}
	turn := &voice{group: group}
	s.mu.Lock()
	s.voices[threadID] = turn
	s.mu.Unlock()
	defer s.endTurn(threadID, turn)
	s.announce(threadID, activity)
	done, err := s.finish(ctx, threadID, job.ID)
	if err != nil {
		return nil, err
	}
	if done.State == acp.StateFailed {
		return nil, fmt.Errorf("%s failed: %s", threadID, done.Error)
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	return slices.Clone(turn.said), nil
}

// finish waits for a turn this service started to end, even when ctx is
// cancelled first.
func (s *Service) finish(ctx context.Context, threadID, jobID string) (acp.Job, error) {
	done, err := s.threads.Wait(context.WithoutCancel(ctx), acp.WaitRequest{Session: jobID, Timeout: turnTimeout})
	if err != nil {
		return acp.Job{}, err
	}
	if done.State == acp.StateStarting || done.State == acp.StateRunning {
		return acp.Job{}, fmt.Errorf("%s is still working", threadID)
	}
	return done, nil
}

func (s *Service) endTurn(threadID string, turn *voice) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.voices[threadID] == turn {
		delete(s.voices, threadID)
	}
}

// announce notes in a thread what woke its bot; the note is best-effort.
func (s *Service) announce(threadID string, activity sessionevents.BotActivityEvent) {
	if threadID == "" {
		return
	}
	if err := s.appendEvent(sessionevents.Event{SessionID: threadID, Type: sessionevents.TypeBotActivity, BotActivity: &activity, At: time.Now().UTC()}); err != nil {
		s.log.Warn("append bot activity failed", "thread", threadID, "error", err)
	}
}

// appendEvent stores event and streams the stored copy, whose seq lets
// clients match it to the same event in history.
func (s *Service) appendEvent(event sessionevents.Event) error {
	events := []sessionevents.Event{event}
	if err := s.store.AppendSessionEvents(event.SessionID, events...); err != nil {
		return err
	}
	s.events.Publish(events[0])
	return nil
}

// resolve finds a bot or group by id, mention target or exact name.
func (s *Service) resolve(ref string) (string, error) {
	ref = strings.TrimPrefix(strings.TrimSpace(ref), "bot:")
	if _, _, err := s.load(ref); err == nil {
		return ref, nil
	}
	sessions, err := s.store.ListSessions(storage.SessionFilter{SourceType: storage.SourceBot})
	if err != nil {
		return "", err
	}
	for _, session := range sessions {
		if strings.EqualFold(session.Title, ref) {
			return session.ID, nil
		}
	}
	return "", fmt.Errorf("no bot or group named %q", ref)
}

// name is how a thread signs its messages: a bot by its name, any other
// thread as Jaz.
func (s *Service) name(threadID string) string {
	if _, session, err := s.load(threadID); err == nil {
		return session.Title
	}
	return "Jaz"
}
