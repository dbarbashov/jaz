package bots

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/wins/jaz/backend/internal/acp"
	"github.com/wins/jaz/backend/internal/sessionevents"
	"github.com/wins/jaz/backend/internal/storage"
)

const turnTimeout = 12 * time.Hour

func (s *Service) AppVisible(threadID string) bool {
	session, err := s.store.LoadSession(threadID)
	return err == nil && (session.Turn == nil || session.Turn.Output == nil)
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
	if err := s.queue.QueueInternalTurn(context.Background(), to, storage.QueuedMessage{
		Text: messagePrompt(sender, text), Output: &storage.TurnOutput{ReplyTo: fromThread},
	}); err != nil {
		return err
	}
	s.announce(fromThread, sessionevents.BotActivityEvent{Kind: "message_sent", Label: session.Title})
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
	message := sessionevents.RoomMessageEvent{Speaker: "bot", BotID: threadID, Name: session.Title, Text: text}
	if session.Turn == nil || session.Turn.Output == nil {
		return s.appendEvent(sessionevents.Event{SessionID: threadID, Type: sessionevents.TypeRoomMessage, RoomMessage: &message, At: time.Now().UTC()})
	}
	output := session.Turn.Output
	if output.ReplyTo != "" {
		return s.store.AppendTurnReply(threadID, text)
	}
	group, groupSession, err := s.load(output.GroupID)
	if err != nil {
		return err
	}
	return s.post(group, groupSession.Title, message)
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

func (s *Service) ask(ctx context.Context, threadID, group, prompt string, activity sessionevents.BotActivityEvent) error {
	job, err := s.threads.StartInternalTurnWhenIdle(ctx, acp.InternalTurnRequest{
		Session: threadID, Message: prompt, AllowSilence: true, Output: &storage.TurnOutput{GroupID: group},
	})
	if err != nil {
		return err
	}
	s.announce(threadID, activity)
	done, err := s.finish(ctx, threadID, job.ID)
	if err != nil {
		return err
	}
	if done.State == acp.StateFailed {
		return fmt.Errorf("%s failed: %s", threadID, done.Error)
	}
	return nil
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
