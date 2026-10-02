package bots

import (
	"context"
	"errors"
	"fmt"
	"math/rand/v2"
	"slices"
	"strings"
	"sync"

	"github.com/charmbracelet/log"
	"github.com/wins/jaz/backend/internal/acp"
	"github.com/wins/jaz/backend/internal/loops"
	"github.com/wins/jaz/backend/internal/sessionevents"
	"github.com/wins/jaz/backend/internal/storage"
)

type Service struct {
	store Store
	// homes is where each bot's own directory lives, named after its id.
	homes    string
	threads  Threads
	routines Routines
	events   Publisher
	log      *log.Logger

	mu        sync.Mutex
	voices    map[string]*voice
	followUps map[string]int
	// waking holds a member's group turns in flight, keyed by group and
	// member, and whether another turn is owed after the current one.
	waking map[string]bool
}

func NewService(store Store, homes string, threads Threads, routines Routines, events Publisher, logger *log.Logger) *Service {
	return &Service{
		store:     store,
		homes:     homes,
		threads:   threads,
		routines:  routines,
		events:    events,
		log:       logger.WithPrefix("bots"),
		voices:    map[string]*voice{},
		followUps: map[string]int{},
		waking:    map[string]bool{},
	}
}

func (s *Service) List() ([]Bot, error) {
	records, err := s.store.ListBots()
	if err != nil {
		return nil, err
	}
	sessions, err := s.store.ListSessions(storage.SessionFilter{SourceType: storage.SourceBot, IncludeSourced: true})
	if err != nil {
		return nil, err
	}
	byThread := make(map[string]storage.BotRecord, len(records))
	for _, record := range records {
		byThread[record.ThreadID] = record
	}
	out := make([]Bot, 0, len(sessions))
	for _, session := range sessions {
		if record, ok := byThread[session.ID]; ok {
			out = append(out, s.view(record, session))
		}
	}
	return out, nil
}

func (s *Service) Load(id string) (Bot, error) {
	record, session, err := s.load(id)
	if err != nil {
		return Bot{}, err
	}
	return s.view(record, session), nil
}

// botModels is what a new bot runs on unless it is given a model: a bot takes
// many short turns, so it starts on a lighter setup than a chat, by agent.
var botModels = map[string]struct{ model, effort string }{
	acp.AgentCodex:  {"gpt-6-luna", "medium"},
	acp.AgentClaude: {"opus[1m]", "medium"},
}

func (s *Service) Create(ctx context.Context, input CreateBot) (Bot, error) {
	name := strings.TrimSpace(input.Name)
	if name == "" {
		return Bot{}, errors.New("name is required")
	}
	avatar := randomAvatar()
	if input.Avatar != nil {
		if err := checkAvatar(*input.Avatar); err != nil {
			return Bot{}, err
		}
		avatar = *input.Avatar
	}
	session, err := s.threads.CreateSession(ctx, acp.SpawnRequest{
		ACPAgent:   strings.TrimSpace(input.Agent),
		Slug:       "bot " + name,
		Title:      name,
		Home:       s.homes,
		Model:      strings.TrimSpace(input.Model),
		SourceType: storage.SourceBot,
	})
	if err != nil {
		return Bot{}, err
	}
	if start, ok := botModels[session.RuntimeRef.Agent]; ok && strings.TrimSpace(input.Model) == "" {
		if err := s.threads.SetModel(ctx, session.ID, start.model, start.effort); err != nil {
			return Bot{}, err
		}
	}
	if err := s.store.UpdateSessionTitle(session.ID, name); err != nil {
		return Bot{}, err
	}
	if err := s.store.SaveBot(storage.BotRecord{ThreadID: session.ID, Kind: KindBot, Shape: avatar.Shape, Color: avatar.Color}); err != nil {
		return Bot{}, err
	}
	return s.Load(session.ID)
}

func (s *Service) CreateGroup(name string, members []string) (Bot, error) {
	name = strings.TrimSpace(name)
	if name == "" {
		return Bot{}, errors.New("name is required")
	}
	members, err := s.memberBots(members)
	if err != nil {
		return Bot{}, err
	}
	session, err := s.store.CreateSession(storage.CreateSession{
		Slug:       "group " + name,
		Title:      name,
		Runtime:    storage.RuntimeACP,
		SourceType: storage.SourceBot,
	})
	if err != nil {
		return Bot{}, err
	}
	avatar := randomAvatar()
	if err := s.store.SaveBot(storage.BotRecord{ThreadID: session.ID, Kind: KindGroup, Shape: avatar.Shape, Color: avatar.Color, Members: members}); err != nil {
		return Bot{}, err
	}
	return s.Load(session.ID)
}

func (s *Service) Update(ctx context.Context, id string, input UpdateBot) (Bot, error) {
	record, _, err := s.load(id)
	if err != nil {
		return Bot{}, err
	}
	if (input.Agent != nil || input.Model != nil) && record.Kind != KindBot {
		return Bot{}, errors.New("only a bot has an agent")
	}
	if input.Members != nil && record.Kind != KindGroup {
		return Bot{}, errors.New("only a group has members")
	}
	if input.ReasoningEffort != "" && input.Model == nil {
		return Bot{}, errors.New("a reasoning effort needs its model")
	}
	name := ""
	if input.Name != nil {
		if name = strings.TrimSpace(*input.Name); name == "" {
			return Bot{}, errors.New("name is required")
		}
	}
	if input.Avatar != nil {
		if err := checkAvatar(*input.Avatar); err != nil {
			return Bot{}, err
		}
		record.Shape, record.Color = input.Avatar.Shape, input.Avatar.Color
	}
	if input.Members != nil {
		if record.Members, err = s.memberBots(*input.Members); err != nil {
			return Bot{}, err
		}
	}
	if input.Agent != nil {
		if err := s.threads.SwitchAgent(ctx, id, strings.TrimSpace(*input.Agent)); err != nil {
			return Bot{}, err
		}
	}
	if input.Model != nil {
		if err := s.threads.SetModel(ctx, id, strings.TrimSpace(*input.Model), strings.TrimSpace(input.ReasoningEffort)); err != nil {
			return Bot{}, err
		}
	}
	if name != "" {
		if err := s.store.UpdateSessionTitle(id, name); err != nil {
			return Bot{}, err
		}
	}
	if err := s.store.SaveBot(record); err != nil {
		return Bot{}, err
	}
	return s.Load(id)
}

// Pin pins exactly ids, in that order, and unpins every other bot and group.
func (s *Service) Pin(ids []string) error {
	return s.store.PinBots(ids)
}

// Delete archives the bot's thread, deletes the routines it owns and takes
// it out of its groups.
func (s *Service) Delete(id string) error {
	if _, _, err := s.load(id); err != nil {
		return err
	}
	records, err := s.store.ListBots()
	if err != nil {
		return err
	}
	for _, group := range records {
		if !slices.Contains(group.Members, id) {
			continue
		}
		group.Members = slices.DeleteFunc(group.Members, func(member string) bool { return member == id })
		if err := s.store.SaveBot(group); err != nil {
			return err
		}
	}
	routines, err := s.routines.List()
	if err != nil {
		return err
	}
	for _, routine := range routines {
		if routine.BotID != id {
			continue
		}
		if err := s.routines.Delete(routine.ID); err != nil {
			return err
		}
	}
	return s.store.SetArchived(id, true)
}

// RoutineOwner returns the bot a new routine belongs to: the bot it names, the
// bot whose thread or subtask created it, or else a new bot of its own.
func (s *Service) RoutineOwner(threadID string, in loops.CreateLoop) (string, error) {
	if in.BotID != "" {
		if !s.isBot(in.BotID) {
			return "", fmt.Errorf("%s is not a bot", in.BotID)
		}
		return in.BotID, nil
	}
	if session, err := s.store.LoadSession(threadID); err == nil && session.SourceType == storage.SourceBotWorker {
		threadID = session.SourceID
	}
	if s.isBot(threadID) {
		return threadID, nil
	}
	name := strings.TrimSpace(in.Name)
	if name == "" {
		name = "Routine bot"
	}
	bot, err := s.Create(context.Background(), CreateBot{Name: name, Agent: in.ACPAgent, Model: in.Model})
	return bot.ID, err
}

// AdoptLoops gives every loop that has no owner and is not on a board a bot
// of its own, so every routine lives with a bot or a board.
func (s *Service) AdoptLoops(ctx context.Context) error {
	routines, err := s.routines.List()
	if err != nil {
		return err
	}
	for _, routine := range routines {
		if routine.BotID != "" || s.routines.OnBoard(routine) {
			continue
		}
		bot, err := s.Create(ctx, CreateBot{Name: routine.Name, Agent: routine.ACPAgent, Model: routine.Model})
		if err != nil {
			return fmt.Errorf("adopt loop %s: %w", routine.ID, err)
		}
		if _, err := s.routines.Update(routine.ID, loops.UpdateLoop{BotID: &bot.ID}); err != nil {
			return fmt.Errorf("adopt loop %s: %w", routine.ID, err)
		}
	}
	return nil
}

func (s *Service) load(id string) (storage.BotRecord, storage.Session, error) {
	record, err := s.store.LoadBot(id)
	if err != nil {
		return storage.BotRecord{}, storage.Session{}, err
	}
	session, err := s.store.LoadSession(id)
	if err != nil {
		return storage.BotRecord{}, storage.Session{}, err
	}
	if session.Archived {
		return storage.BotRecord{}, storage.Session{}, storage.ErrBotNotFound
	}
	return record, session, nil
}

func (s *Service) isBot(id string) bool {
	record, _, err := s.load(id)
	return err == nil && record.Kind == KindBot
}

func (s *Service) view(record storage.BotRecord, session storage.Session) Bot {
	bot := Bot{
		ID:        session.ID,
		Kind:      record.Kind,
		Name:      session.Title,
		Avatar:    Avatar{Shape: record.Shape, Color: record.Color},
		Pinned:    record.Pinned,
		Unread:    session.Unread,
		Status:    session.Status,
		UpdatedAt: session.UpdatedAt,
		Members:   record.Members,
		Preview:   s.preview(session.ID, record.Kind == KindGroup),
	}
	if record.Kind == KindGroup {
		return bot
	}
	bot.Model = session.Model
	bot.ReasoningEffort = session.ReasoningEffort
	if ref := session.RuntimeRef; ref != nil {
		bot.Agent = ref.Agent
	}
	return bot
}

// preview is the newest chat message in a bot's or group's thread; a group's
// preview names its speaker.
func (s *Service) preview(threadID string, group bool) string {
	event, ok, err := s.store.LoadLatestSessionEvent(threadID, sessionevents.TypeRoomMessage)
	if err != nil || !ok || event.RoomMessage == nil {
		return ""
	}
	text := event.RoomMessage.Text
	if group {
		text = event.RoomMessage.Name + ": " + text
	}
	text = strings.Join(strings.Fields(text), " ")
	if runes := []rune(text); len(runes) > 140 {
		return string(runes[:140]) + "…"
	}
	return text
}

func (s *Service) memberBots(ids []string) ([]string, error) {
	members := make([]string, 0, len(ids))
	for _, id := range ids {
		if !s.isBot(id) {
			return nil, fmt.Errorf("member %s is not a bot", id)
		}
		if !slices.Contains(members, id) {
			members = append(members, id)
		}
	}
	if len(members) < 2 {
		return nil, errors.New("a group needs at least two bots")
	}
	return members, nil
}

// randomAvatar skips the neutral white and gray at the ends of colors.
func randomAvatar() Avatar {
	vivid := colors[1 : len(colors)-1]
	return Avatar{Shape: shapes[rand.IntN(len(shapes))], Color: vivid[rand.IntN(len(vivid))]}
}

func checkAvatar(avatar Avatar) error {
	if !slices.Contains(shapes, avatar.Shape) || !slices.Contains(colors, avatar.Color) {
		return fmt.Errorf("unsupported avatar %s/%s", avatar.Shape, avatar.Color)
	}
	return nil
}
