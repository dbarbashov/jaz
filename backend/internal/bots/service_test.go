package bots

import (
	"cmp"
	"context"
	"errors"
	"slices"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/charmbracelet/log"
	"github.com/wins/jaz/backend/internal/acp"
	"github.com/wins/jaz/backend/internal/loops"
	"github.com/wins/jaz/backend/internal/sessionevents"
	"github.com/wins/jaz/backend/internal/storage"
)

type fakeWorld struct {
	mu        sync.Mutex
	records   map[string]storage.BotRecord
	sessions  map[string]storage.Session
	events    map[string][]sessionevents.Event
	prompts   map[string][]string
	replies   map[string][]string
	created   []acp.SpawnRequest
	models    map[string]string
	loops     []loops.Loop
	published []sessionevents.Event
	held      map[string]chan struct{}
	// busy holds a thread's next turn back, as another turn running there
	// would.
	busy    map[string]chan struct{}
	running map[string]bool
	steered map[string][]string
	// steerless makes every thread's agent unable to take messages mid-turn.
	steerless bool
	service   *Service
}

func newFakeWorld() *fakeWorld {
	return &fakeWorld{
		records:  map[string]storage.BotRecord{},
		sessions: map[string]storage.Session{},
		events:   map[string][]sessionevents.Event{},
		prompts:  map[string][]string{},
		replies:  map[string][]string{},
		models:   map[string]string{},
		held:     map[string]chan struct{}{},
		busy:     map[string]chan struct{}{},
		running:  map[string]bool{},
		steered:  map[string][]string{},
	}
}

func (w *fakeWorld) SaveBot(record storage.BotRecord) error {
	w.mu.Lock()
	defer w.mu.Unlock()
	w.records[record.ThreadID] = record
	return nil
}

func (w *fakeWorld) LoadBot(id string) (storage.BotRecord, error) {
	w.mu.Lock()
	defer w.mu.Unlock()
	record, ok := w.records[id]
	if !ok {
		return storage.BotRecord{}, storage.ErrBotNotFound
	}
	return record, nil
}

func (w *fakeWorld) ListBots() ([]storage.BotRecord, error) {
	w.mu.Lock()
	defer w.mu.Unlock()
	out := make([]storage.BotRecord, 0, len(w.records))
	for _, record := range w.records {
		out = append(out, record)
	}
	return out, nil
}

func (w *fakeWorld) PinBots(ids []string) error {
	w.mu.Lock()
	defer w.mu.Unlock()
	for id, record := range w.records {
		record.Pinned = slices.Index(ids, id) + 1
		w.records[id] = record
	}
	return nil
}

func (w *fakeWorld) CreateSession(input storage.CreateSession) (storage.Session, error) {
	w.mu.Lock()
	defer w.mu.Unlock()
	session := storage.Session{ID: "thread-" + input.Slug, Title: input.Title, SourceType: input.SourceType}
	w.sessions[session.ID] = session
	return session, nil
}

func (w *fakeWorld) LoadSession(id string) (storage.Session, error) {
	w.mu.Lock()
	defer w.mu.Unlock()
	session, ok := w.sessions[id]
	if !ok {
		return storage.Session{}, storage.ErrBotNotFound
	}
	return session, nil
}

func (w *fakeWorld) ListSessions(storage.SessionFilter) ([]storage.Session, error) {
	w.mu.Lock()
	defer w.mu.Unlock()
	out := make([]storage.Session, 0, len(w.sessions))
	for _, session := range w.sessions {
		out = append(out, session)
	}
	return out, nil
}

func (w *fakeWorld) UpdateSessionTitle(id, title string) error {
	w.mu.Lock()
	defer w.mu.Unlock()
	session := w.sessions[id]
	session.Title = title
	w.sessions[id] = session
	return nil
}

func (w *fakeWorld) SetArchived(id string, archived bool) error {
	w.mu.Lock()
	defer w.mu.Unlock()
	session := w.sessions[id]
	session.Archived = archived
	w.sessions[id] = session
	return nil
}

func (w *fakeWorld) LoadSessionEvents(id string) ([]sessionevents.Event, error) {
	w.mu.Lock()
	defer w.mu.Unlock()
	return append([]sessionevents.Event(nil), w.events[id]...), nil
}

func (w *fakeWorld) AppendSessionEvents(id string, events ...sessionevents.Event) error {
	w.mu.Lock()
	defer w.mu.Unlock()
	for i := range events {
		events[i].Seq = int64(len(w.events[id]) + 1)
		w.events[id] = append(w.events[id], events[i])
	}
	return nil
}

func (w *fakeWorld) LoadLatestSessionEvent(id, eventType string) (sessionevents.Event, bool, error) {
	w.mu.Lock()
	defer w.mu.Unlock()
	for i := len(w.events[id]) - 1; i >= 0; i-- {
		if w.events[id][i].Type == eventType {
			return w.events[id][i], true, nil
		}
	}
	return sessionevents.Event{}, false, nil
}

func (w *fakeWorld) Publish(event sessionevents.Event) {
	w.mu.Lock()
	defer w.mu.Unlock()
	w.published = append(w.published, event)
}

func (w *fakeWorld) List() ([]loops.Loop, error) {
	return w.loops, nil
}

func (w *fakeWorld) Update(id string, input loops.UpdateLoop) (loops.Loop, error) {
	for i := range w.loops {
		if w.loops[i].ID == id && input.BotID != nil {
			w.loops[i].BotID = *input.BotID
			return w.loops[i], nil
		}
	}
	return loops.Loop{}, nil
}

func (w *fakeWorld) Delete(string) error {
	return nil
}

func (w *fakeWorld) OnBoard(loop loops.Loop) bool {
	return strings.HasPrefix(loop.ID, "widget-")
}

type fakeThreads struct {
	world *fakeWorld
}

func (t fakeThreads) CreateSession(_ context.Context, req acp.SpawnRequest) (storage.Session, error) {
	t.world.mu.Lock()
	t.world.created = append(t.world.created, req)
	t.world.mu.Unlock()
	session, err := t.world.CreateSession(storage.CreateSession{Slug: req.Slug, Title: req.Title, SourceType: req.SourceType})
	// Like the manager, an omitted agent is the default one.
	session.RuntimeRef = &storage.RuntimeRef{Agent: cmp.Or(req.ACPAgent, acp.AgentCodex)}
	t.world.mu.Lock()
	t.world.sessions[session.ID] = session
	t.world.mu.Unlock()
	return session, err
}

func (t fakeThreads) StartInternalTurnWhenIdle(_ context.Context, req acp.InternalTurnRequest) (acp.Job, error) {
	t.world.mu.Lock()
	busy := t.world.busy[req.Session]
	t.world.mu.Unlock()
	if busy != nil {
		<-busy
	}
	t.world.mu.Lock()
	defer t.world.mu.Unlock()
	t.world.prompts[req.Session] = append(t.world.prompts[req.Session], req.Message)
	t.world.running[req.Session] = true
	return acp.Job{ID: req.Session}, nil
}

// SteerInternal hands a message to the thread's running turn, as the manager
// does, failing when no turn runs or its agent cannot take it.
func (t fakeThreads) SteerInternal(_ context.Context, session, message string) (acp.Job, error) {
	t.world.mu.Lock()
	defer t.world.mu.Unlock()
	if !t.world.running[session] {
		return acp.Job{}, errors.New("turn ended")
	}
	if t.world.steerless {
		return acp.Job{}, acp.ErrSteeringUnsupported
	}
	t.world.steered[session] = append(t.world.steered[session], message)
	return acp.Job{ID: session}, nil
}

// Wait runs the turn: the bot sends its scripted message, if any, the way it
// would call send_message.
func (t fakeThreads) Wait(_ context.Context, req acp.WaitRequest) (acp.Job, error) {
	t.world.mu.Lock()
	reply := ""
	if queue := t.world.replies[req.Session]; len(queue) > 0 {
		reply = queue[0]
		t.world.replies[req.Session] = queue[1:]
	}
	held := t.world.held[req.Session]
	t.world.mu.Unlock()
	if held != nil {
		<-held
	}
	if reply != "" {
		if err := t.world.service.Say(req.Session, reply); err != nil {
			return acp.Job{}, err
		}
	}
	t.world.mu.Lock()
	t.world.running[req.Session] = false
	t.world.mu.Unlock()
	return acp.Job{ID: req.Session, State: acp.StateIdle, Assistant: "private notes"}, nil
}

func (t fakeThreads) SetModel(_ context.Context, sessionID, model, effort string) error {
	t.world.mu.Lock()
	defer t.world.mu.Unlock()
	t.world.models[sessionID] = model + "/" + effort
	return nil
}

func (fakeThreads) SwitchAgent(context.Context, string, string) error {
	return nil
}

func newTestService(world *fakeWorld) *Service {
	world.service = NewService(world, "/bots", fakeThreads{world: world}, world, world, log.New(nil))
	return world.service
}

func (w *fakeWorld) addBot(id, name string) {
	w.sessions[id] = storage.Session{ID: id, Title: name}
	w.records[id] = storage.BotRecord{ThreadID: id, Kind: KindBot}
}

func (w *fakeWorld) roomMessages(id string) []string {
	w.mu.Lock()
	defer w.mu.Unlock()
	var out []string
	for _, event := range w.events[id] {
		if event.RoomMessage != nil {
			out = append(out, event.RoomMessage.Name+": "+event.RoomMessage.Text)
		}
	}
	return out
}

func (w *fakeWorld) promptCount(id string) int {
	w.mu.Lock()
	defer w.mu.Unlock()
	return len(w.prompts[id])
}

func waitUntil(t *testing.T, check func() bool) {
	t.Helper()
	deadline := time.Now().Add(2 * time.Second)
	for !check() {
		if time.Now().After(deadline) {
			t.Fatal("condition not reached")
		}
		time.Sleep(5 * time.Millisecond)
	}
}

func TestRoutineOwnerChecksNamedBotsKeepsBotThreadAndGivesOtherThreadsANewBot(t *testing.T) {
	world := newFakeWorld()
	world.addBot("gimli", "Gimli")
	service := newTestService(world)

	owner, err := service.RoutineOwner("gimli", loops.CreateLoop{Name: "Digest"})
	if err != nil || owner != "gimli" {
		t.Fatalf("owner from a bot thread = %q, %v", owner, err)
	}
	owner, err = service.RoutineOwner("chat-thread", loops.CreateLoop{Name: "Morning triage", ACPAgent: "claude"})
	if err != nil {
		t.Fatal(err)
	}
	record, err := world.LoadBot(owner)
	if err != nil || record.Kind != KindBot {
		t.Fatalf("new owner record = %+v, %v", record, err)
	}
	if len(world.created) != 1 || world.created[0].SourceType != storage.SourceBot || world.created[0].ACPAgent != "claude" || world.created[0].Title != "Morning triage" {
		t.Fatalf("created threads = %+v", world.created)
	}
	if owner, err := service.RoutineOwner("chat-thread", loops.CreateLoop{BotID: "gimli"}); err != nil || owner != "gimli" {
		t.Fatalf("named owner = %q, %v", owner, err)
	}
	world.sessions["research"] = storage.Session{ID: "research", SourceType: storage.SourceBotWorker, SourceID: "gimli"}
	if owner, err := service.RoutineOwner("research", loops.CreateLoop{Name: "Watch"}); err != nil || owner != "gimli" || len(world.created) != 1 {
		t.Fatalf("a subtask's routine went to %q (%v), %d bots created", owner, err, len(world.created))
	}
	world.sessions["crew"] = storage.Session{ID: "crew", Title: "Crew"}
	world.records["crew"] = storage.BotRecord{ThreadID: "crew", Kind: KindGroup}
	for _, named := range []string{"crew", "missing"} {
		if _, err := service.RoutineOwner("gimli", loops.CreateLoop{BotID: named}); err == nil {
			t.Fatalf("routine was given to %q, which is not a bot", named)
		}
	}
}

func TestMembersFollowUpOnlyWhenMentioned(t *testing.T) {
	world := newFakeWorld()
	world.addBot("a", "Research")
	world.addBot("b", "Marketing")
	service := newTestService(world)
	group, err := service.CreateGroup("Launch", []string{"a", "b"})
	if err != nil {
		t.Fatal(err)
	}
	world.replies["b"] = []string{"[@Research] can you check the numbers?"}
	world.replies["a"] = []string{"Checked."}
	release := make(chan struct{})
	world.held["a"] = release

	if err := service.Post(group.ID, "[@Marketing](bot:b) where is the launch draft?"); err != nil {
		t.Fatal(err)
	}
	// Research answers after Marketing's turn has ended, as a real agent's
	// slower turn would, so the answer reaches Marketing only if it is
	// addressed.
	waitUntil(t, func() bool { return world.promptCount("a") == 1 && service.AppVisible("b") })
	close(release)
	waitUntil(t, func() bool { return slices.Contains(world.roomMessages(group.ID), "Research: Checked.") })
	time.Sleep(20 * time.Millisecond)
	if world.promptCount("a") != 1 || world.promptCount("b") != 1 {
		t.Fatalf("turns: Research %d, Marketing %d", world.promptCount("a"), world.promptCount("b"))
	}
	world.mu.Lock()
	prompt := world.prompts["a"][0]
	world.mu.Unlock()
	if !strings.Contains(prompt, "can you check the numbers?") {
		t.Fatalf("follow-up prompt misses the mention:\n%s", prompt)
	}
}

// progressDuringTurn has Research, working on its turn, mention Marketing with
// a first finding and then post two more while Marketing works on its own
// turn, and returns once Marketing's agent has taken or refused them.
func progressDuringTurn(t *testing.T, world *fakeWorld, service *Service, releaseMarketing chan struct{}) string {
	t.Helper()
	group, err := service.CreateGroup("Launch", []string{"a", "b"})
	if err != nil {
		t.Fatal(err)
	}
	world.held["a"] = make(chan struct{})
	world.held["b"] = releaseMarketing
	if err := service.Post(group.ID, "[@Research](bot:a) dig into pricing"); err != nil {
		t.Fatal(err)
	}
	waitUntil(t, func() bool { return !service.AppVisible("a") })
	say := func(text string) {
		if err := service.Say("a", text); err != nil {
			t.Fatal(err)
		}
	}
	say("[@Marketing](bot:b) first finding")
	waitUntil(t, func() bool { return !service.AppVisible("b") })
	say("second finding")
	say("third finding")
	return group.ID
}

func TestPostsDuringAMembersTurnReachItBeforeTheTurnEnds(t *testing.T) {
	world := newFakeWorld()
	world.addBot("a", "Research")
	world.addBot("b", "Marketing")
	service := newTestService(world)
	release := make(chan struct{})
	progressDuringTurn(t, world, service, release)

	steered := func() string {
		world.mu.Lock()
		defer world.mu.Unlock()
		return strings.Join(world.steered["b"], "\n")
	}
	waitUntil(t, func() bool {
		return strings.Contains(steered(), "Research: second finding") && strings.Contains(steered(), "Research: third finding")
	})
	if strings.Count(steered(), "second finding") != 1 || strings.Contains(steered(), "first finding") {
		t.Fatalf("Marketing was handed:\n%s", steered())
	}
	close(release)
	close(world.held["a"])
	waitUntil(t, func() bool {
		service.mu.Lock()
		defer service.mu.Unlock()
		return len(service.waking) == 0
	})
	if world.promptCount("b") != 1 {
		world.mu.Lock()
		defer world.mu.Unlock()
		t.Fatalf("Marketing took %d turns: %q", len(world.prompts["b"]), world.prompts["b"])
	}
}

func TestPostsAnAgentCannotTakeMidTurnFoldIntoOneMoreTurn(t *testing.T) {
	world := newFakeWorld()
	world.addBot("a", "Research")
	world.addBot("b", "Marketing")
	world.steerless = true
	world.replies["b"] = []string{"Looking at the first one."}
	service := newTestService(world)
	release := make(chan struct{})
	progressDuringTurn(t, world, service, release)

	close(release)
	waitUntil(t, func() bool { return world.promptCount("b") == 2 })
	close(world.held["a"])
	world.mu.Lock()
	defer world.mu.Unlock()
	if next := world.prompts["b"][1]; !strings.Contains(next, "second finding") || !strings.Contains(next, "third finding") || strings.Contains(next, "first finding") {
		t.Fatalf("Marketing's next turn:\n%s", next)
	}
}

func TestPostsBeforeAMembersTurnStartsReachItAsItStarts(t *testing.T) {
	world := newFakeWorld()
	world.addBot("a", "Research")
	world.addBot("b", "Marketing")
	service := newTestService(world)
	group, err := service.CreateGroup("Launch", []string{"a", "b"})
	if err != nil {
		t.Fatal(err)
	}
	idle := make(chan struct{})
	world.busy["b"] = idle
	release := make(chan struct{})
	world.held["b"] = release

	if err := service.Message("a", group.ID, "[@Marketing](bot:b) draft the launch post"); err != nil {
		t.Fatal(err)
	}
	waitUntil(t, func() bool {
		service.mu.Lock()
		defer service.mu.Unlock()
		return len(service.waking) == 1
	})
	if err := service.Message("a", group.ID, "[@Marketing](bot:b) keep it under 100 words"); err != nil {
		t.Fatal(err)
	}
	close(idle)
	waitUntil(t, func() bool {
		world.mu.Lock()
		defer world.mu.Unlock()
		return len(world.steered["b"]) == 1
	})
	close(release)
	waitUntil(t, func() bool {
		service.mu.Lock()
		defer service.mu.Unlock()
		return len(service.waking) == 0
	})
	world.mu.Lock()
	defer world.mu.Unlock()
	if len(world.prompts["b"]) != 1 || !strings.Contains(world.prompts["b"][0], "draft the launch post") || !strings.Contains(world.steered["b"][0], "keep it under 100 words") {
		t.Fatalf("turns %q, handed mid-turn %q", world.prompts["b"], world.steered["b"])
	}
}

func TestMentionPingPongStopsAtTheFollowUpCap(t *testing.T) {
	world := newFakeWorld()
	world.addBot("a", "Research")
	world.addBot("b", "Marketing")
	service := newTestService(world)
	group, err := service.CreateGroup("Launch", []string{"a", "b"})
	if err != nil {
		t.Fatal(err)
	}
	for range 20 {
		world.replies["a"] = append(world.replies["a"], "[@Marketing](bot:b) you?")
		world.replies["b"] = append(world.replies["b"], "[@Research](bot:a) no, you?")
	}
	turns := func() int { return world.promptCount("a") + world.promptCount("b") }

	if err := service.Post(group.ID, "who goes first?"); err != nil {
		t.Fatal(err)
	}
	waitUntil(t, func() bool {
		before := turns()
		time.Sleep(30 * time.Millisecond)
		return before > 2 && turns() == before
	})
	if turns() > 2+maxFollowUps {
		t.Fatalf("turns = %d, cap is %d", turns(), 2+maxFollowUps)
	}
}

func TestASlowMemberDoesNotHoldUpTheOthers(t *testing.T) {
	world := newFakeWorld()
	world.addBot("a", "Research")
	world.addBot("b", "Marketing")
	service := newTestService(world)
	group, err := service.CreateGroup("Launch", []string{"a", "b"})
	if err != nil {
		t.Fatal(err)
	}
	release := make(chan struct{})
	world.held["a"] = release
	world.replies["a"] = []string{"Here is a meme."}
	world.replies["b"] = []string{"Quick one."}

	if err := service.Post(group.ID, "a good meme please"); err != nil {
		t.Fatal(err)
	}
	waitUntil(t, func() bool { return slices.Contains(world.roomMessages(group.ID), "Marketing: Quick one.") })
	close(release)
	waitUntil(t, func() bool { return slices.Contains(world.roomMessages(group.ID), "Research: Here is a meme.") })
}

func TestNewBotsStartOnALightModelUnlessGivenOne(t *testing.T) {
	world := newFakeWorld()
	service := newTestService(world)

	for _, tc := range []struct{ agent, model, want string }{
		{"", "", "gpt-6-luna/medium"},
		{acp.AgentClaude, "", "opus[1m]/medium"},
		{acp.AgentClaude, "sonnet", ""},
	} {
		bot, err := service.Create(t.Context(), CreateBot{Name: "Bot " + tc.agent + tc.model, Agent: tc.agent, Model: tc.model})
		if err != nil {
			t.Fatal(err)
		}
		if got := world.models[bot.ID]; got != tc.want {
			t.Fatalf("%s bot %q starts on %q, want %q", tc.agent, tc.model, got, tc.want)
		}
	}
}

func TestUpdateChangesNothingWhenAnyPartIsInvalid(t *testing.T) {
	world := newFakeWorld()
	world.addBot("gimli", "Gimli")
	service := newTestService(world)

	name := "Thorin"
	if _, err := service.Update(t.Context(), "gimli", UpdateBot{Name: &name, Avatar: &Avatar{Shape: "blob", Color: "plaid"}}); err == nil {
		t.Fatal("an unsupported avatar was accepted")
	}
	if title := world.sessions["gimli"].Title; title != "Gimli" {
		t.Fatalf("a rejected update renamed the bot to %q", title)
	}
}

func TestRoutineRunsAsAnnouncedTurnInItsBotsThread(t *testing.T) {
	world := newFakeWorld()
	world.addBot("gimli", "Gimli")
	service := newTestService(world)

	job, err := service.RunRoutine(t.Context(), "gimli", "Digest", "summarise the inbox")
	if err != nil || job.State != acp.StateIdle {
		t.Fatalf("routine turn = %+v, %v", job, err)
	}
	world.mu.Lock()
	defer world.mu.Unlock()
	if prompts := world.prompts["gimli"]; len(prompts) != 1 || !strings.HasPrefix(prompts[0], "summarise the inbox") {
		t.Fatalf("routine prompts = %q", prompts)
	}
	if events := world.events["gimli"]; len(events) != 1 || events[0].BotActivity == nil || events[0].BotActivity.Kind != "routine" || events[0].BotActivity.Label != "Digest" {
		t.Fatalf("bot chat events = %+v", events)
	}
}

func TestMessageRelaysReplyToSender(t *testing.T) {
	world := newFakeWorld()
	world.addBot("gimli", "Gimli")
	world.addBot("egg", "dr eggbot")
	service := newTestService(world)
	world.replies["egg"] = []string{"Grok on the temporal harness."}
	held := make(chan struct{})
	world.held["egg"] = held

	if err := service.Message("gimli", "egg", "What model do you run on?"); err != nil {
		t.Fatal(err)
	}
	waitUntil(t, func() bool { return !service.AppVisible("egg") })
	if !service.AppVisible("gimli") {
		t.Error("the sender's user chat was hidden with the private recipient turn")
	}
	close(held)
	waitUntil(t, func() bool { return world.promptCount("gimli") == 1 })
	if !service.AppVisible("egg") {
		t.Error("the finished private turn kept later app results hidden")
	}
	world.mu.Lock()
	defer world.mu.Unlock()
	if asked := world.prompts["egg"][0]; !strings.HasPrefix(asked, "[message from Gimli]") || !strings.Contains(asked, "What model do you run on?") {
		t.Fatalf("recipient prompt = %q", asked)
	}
	if relayed := world.prompts["gimli"][0]; !strings.HasPrefix(relayed, "[reply from dr eggbot]") || !strings.Contains(relayed, "temporal harness") || strings.Contains(relayed, "private notes") {
		t.Fatalf("relayed reply = %q", relayed)
	}
	if len(world.published) == 0 {
		t.Fatal("nothing was published")
	}
	for _, event := range world.published {
		if event.RoomMessage != nil {
			t.Fatalf("an answer to a bot reached a chat: %+v", event.RoomMessage)
		}
		if event.Seq == 0 {
			t.Fatalf("published %s without the seq clients match history by", event.Type)
		}
	}
}

func TestSayOutsideAServiceTurnReachesTheBotsOwnChat(t *testing.T) {
	world := newFakeWorld()
	world.addBot("gimli", "Gimli")
	service := newTestService(world)

	if err := service.Say("gimli", "  Moved it to In Progress.  "); err != nil {
		t.Fatal(err)
	}
	if got := strings.Join(world.roomMessages("gimli"), "\n"); got != "Gimli: Moved it to In Progress." {
		t.Fatalf("own chat = %q", got)
	}
	if bot, err := service.Load("gimli"); err != nil || bot.Preview != "Moved it to In Progress." {
		t.Fatalf("preview = %q, %v", bot.Preview, err)
	}
	if err := service.Say("plain-chat", "hi"); err == nil {
		t.Fatal("a thread that is not a bot sent a bot message")
	}
}

func TestAdoptLoopsGivesEachOwnerlessLoopItsOwnBotExceptBoardWidgets(t *testing.T) {
	world := newFakeWorld()
	world.addBot("gimli", "Gimli")
	world.loops = []loops.Loop{
		{ID: "loop-1", Name: "Morning triage", ACPAgent: "codex", Directory: "triage"},
		{ID: "loop-2", Name: "Digest", BotID: "gimli"},
		{ID: "widget-loop", Name: "world-clocks"},
	}
	if err := newTestService(world).AdoptLoops(context.Background()); err != nil {
		t.Fatal(err)
	}
	adopted := world.loops[0].BotID
	record, err := world.LoadBot(adopted)
	if err != nil || record.Kind != KindBot || world.sessions[adopted].Title != "Morning triage" {
		t.Fatalf("adopted by %q: %+v, %v", adopted, record, err)
	}
	if len(world.created) != 1 || world.created[0].ACPAgent != "codex" || world.created[0].Home != "/bots" || world.created[0].Directory != "" {
		t.Fatalf("created bots = %+v", world.created)
	}
	if world.loops[1].BotID != "gimli" {
		t.Fatal("an owned loop changed owner")
	}
	if world.loops[2].BotID != "" {
		t.Fatal("a board widget was given a bot")
	}
}
