package mafia

import (
	"encoding/json"
	"fmt"
	"math/rand"
	"regexp"
	"sort"
	"strconv"
	"sync"
	"time"

	"github.com/cinnabar-games/backend/internal/auth"
	"github.com/cinnabar-games/backend/internal/chat"
	"github.com/cinnabar-games/backend/internal/db"
	"github.com/cinnabar-games/backend/internal/lobby"
	"github.com/cinnabar-games/backend/internal/protocol"
	"github.com/lonng/nano/component"
	"github.com/lonng/nano/session"
)

const (
	MinPlayers = 5
	MaxPlayers = 16

	nightMs        = 120000
	discussionMs   = 600000
	voteMs         = 90000
	nightResultMs  = 5000
	voteResultMs   = 5000

	phaseSetup          = "setup"
	phaseNight          = "night"
	phaseNightResult    = "night_result"
	phaseDayDiscussion  = "day_discussion"
	phaseDayVote        = "day_vote"
	phaseDayVoteResult  = "day_vote_result"
	phaseGameOver       = "game_over"

	roleMafia      = "mafia"
	roleCivilian   = "civilian"
	roleDetective  = "detective"
	roleDoctor     = "doctor"
	roleSheriff    = "sheriff"
	roleGodfather  = "godfather"
	roleJester     = "jester"

	winMafia     = "mafia"
	winCivilian  = "civilians"
	winJester    = "jester"
)

type Player struct {
	ID       int64  `json:"id"`
	Username string `json:"username"`
	Alive    bool   `json:"alive"`
	Role     string `json:"role,omitempty"`
}

type GameConfig struct {
	NumMafia           int  `json:"num_mafia"`
	HasDetective       bool `json:"has_detective"`
	HasDoctor          bool `json:"has_doctor"`
	HasSheriff         bool `json:"has_sheriff"`
	HasGodfather       bool `json:"has_godfather"`
	HasJester          bool `json:"has_jester"`
	RevealOnDeath      bool `json:"reveal_on_death"`
	AnnounceSavedPlayer bool `json:"announce_saved_player"`
	NightTimerMs       int  `json:"night_timer_ms"`
	DiscussionTimerMs  int  `json:"discussion_timer_ms"`
	VoteTimerMs        int  `json:"vote_timer_ms"`
}

type VoteTally struct {
	PlayerID  int64  `json:"player_id"`
	Username  string `json:"username"`
	VoteCount int    `json:"vote_count"`
	Role      string `json:"role,omitempty"`
}

type NightActionData struct {
	MafiaTarget     int64 `json:"mafia_target"`
	DetectiveTarget int64 `json:"detective_target"`
	DoctorTarget    int64 `json:"doctor_target"`
	SheriffTarget   int64 `json:"sheriff_target"`
	SheriffSkip     bool  `json:"sheriff_skip"`
}

type MafiaMarkPayload struct {
	TargetIDs []int64 `json:"target_ids"`
}

type InvestigatePayload struct {
	TargetID       int64  `json:"target_id"`
	TargetUsername string `json:"target_username"`
	Result         string `json:"result"`
}

type NightResultPayload struct {
	KilledPlayers  []Player `json:"killed_players"`
	NoKill         bool     `json:"no_kill"`
	SheriffDied    bool     `json:"sheriff_died"`
	SheriffKilled  *Player  `json:"sheriff_killed,omitempty"`
	SavedByDoctor  bool     `json:"saved_by_doctor"`
}

type RolePayload struct {
	Role           string   `json:"role"`
	KnownTeammates []Player `json:"known_teammates"`
}

type GameState struct {
	Players     []Player `json:"players"`
	DeadPlayers []Player `json:"dead_players"`

	Phase string `json:"phase"`
	Round int    `json:"round"`

	Config GameConfig `json:"config"`

	PhaseEndsAt int64 `json:"phase_ends_at"`

	NightReadyRoles []string `json:"night_ready_roles"`
	MafiaReadyCount int      `json:"mafia_ready_count"`

	LastNightKills []Player `json:"last_night_kills"`
	LastNightSaved bool     `json:"last_night_saved"`
	LastNightEvent string   `json:"last_night_event"`

	VotesIn      int         `json:"votes_in"`
	TotalVoters  int         `json:"total_voters"`
	VoteResults  []VoteTally `json:"vote_results,omitempty"`
	IsRevote     bool        `json:"is_revote"`
	VoteCandidates []int64   `json:"vote_candidates,omitempty"`

	SheriffUsedAbility bool `json:"sheriff_used_ability"`
	JesterWon          bool `json:"jester_won"`

	Status    string `json:"status"`
	Winner    string `json:"winner"`
	WinReason string `json:"win_reason"`
	LastEvent string `json:"last_event"`

	HostID int64 `json:"host_id"`

	History []RoundLog `json:"history"`

	TeamCounts *TeamCounts `json:"team_counts,omitempty"`

	// private
	roles             map[int64]string
	nightActions      map[int64]*NightActionData
	detectiveLog      []InvestigatePayload
	doctorLastSave    int64
	mafiaLastTarget   int64
	votes             map[int64]int64
	onDisconnect      func(playerID int64)
}

func (s *GameState) GetStatus() string { return s.Status }


type TeamCounts struct {
	MafiaAlive   int `json:"mafia_alive"`
	CivAlive     int `json:"civ_alive"`
	NeutralAlive int `json:"neutral_alive"`
	TotalDead    int `json:"total_dead"`
}

type RoundLog struct {
	Round      int    `json:"round"`
	Phase      string `json:"phase"`
	TargetID   int64  `json:"target_id"`
	TargetName string `json:"target_name"`
	Detail     string `json:"detail"`
}

func (s *GameState) MarshalState() ([]byte, error) {
	type Alias GameState
	return json.Marshal(&struct {
		Roles           map[string]string `json:"_roles"`
		NightActions    map[string]*NightActionData `json:"_night_actions,omitempty"`
		DetectiveLog    []InvestigatePayload `json:"_detective_log,omitempty"`
		DoctorLastSave  int64 `json:"_doctor_last_save"`
		MafiaLastTarget int64 `json:"_mafia_last_target"`
		Votes           map[string]int64 `json:"_votes,omitempty"`
		*Alias
	}{
		Roles:           rolesMapToJSON(s.roles),
		NightActions:    nightActionsToJSON(s.nightActions),
		DetectiveLog:    s.detectiveLog,
		DoctorLastSave:  s.doctorLastSave,
		MafiaLastTarget: s.mafiaLastTarget,
		Votes:           votesMapToJSON(s.votes),
		Alias:           (*Alias)(s),
	})
}

func (s *GameState) UnmarshalJSON(data []byte) error {
	type Alias GameState
	aux := &struct {
		Roles           map[string]string `json:"_roles"`
		NightActions    map[string]*NightActionData `json:"_night_actions,omitempty"`
		DetectiveLog    []InvestigatePayload `json:"_detective_log,omitempty"`
		DoctorLastSave  int64 `json:"_doctor_last_save"`
		MafiaLastTarget int64 `json:"_mafia_last_target"`
		Votes           map[string]int64 `json:"_votes,omitempty"`
		*Alias
	}{Alias: (*Alias)(s)}
	if err := json.Unmarshal(data, aux); err != nil {
		return err
	}
	s.roles = rolesMapFromJSON(aux.Roles)
	s.nightActions = nightActionsFromJSON(aux.NightActions)
	s.detectiveLog = aux.DetectiveLog
	s.doctorLastSave = aux.DoctorLastSave
	s.mafiaLastTarget = aux.MafiaLastTarget
	s.votes = votesMapFromJSON(aux.Votes)
	return nil
}

type RoomReq struct {
	RoomID string `json:"room_id"`
}

type ConfigureReq struct {
	RoomID string     `json:"room_id"`
	Config GameConfig `json:"config"`
}

type NightActionReq struct {
	RoomID    string `json:"room_id"`
	TargetID  int64  `json:"target_id"`
	Skip      bool   `json:"skip,omitempty"`
}

type CastVoteReq struct {
	RoomID   string `json:"room_id"`
	TargetID int64  `json:"target_id"`
}

type Mafia struct {
	component.Base
	lobby  *lobby.Lobby
	chat   *chat.Chat
	mu     sync.Mutex
	timers map[string]*time.Timer
}

func NewMafia(lob *lobby.Lobby, chatComp *chat.Chat) *Mafia {
	return &Mafia{
		lobby:  lob,
		chat:   chatComp,
		timers: make(map[string]*time.Timer),
	}
}

func BlankState() *GameState {
	return &GameState{
		Players:     []Player{},
		DeadPlayers: []Player{},
		History:     []RoundLog{},
		roles:       make(map[int64]string),
		nightActions: make(map[int64]*NightActionData),
		detectiveLog: []InvestigatePayload{},
		votes:       make(map[int64]int64),
		Status:      "waiting",
	}
}

func defaultConfig() GameConfig {
	return GameConfig{
		NumMafia:            1,
		HasDetective:        false,
		HasDoctor:           true,
		HasSheriff:          false,
		HasGodfather:        false,
		HasJester:           false,
		RevealOnDeath:       true,
		AnnounceSavedPlayer: true,
		NightTimerMs:        nightMs,
		DiscussionTimerMs:   discussionMs,
		VoteTimerMs:         voteMs,
	}
}

func (g *Mafia) InitGame(room *lobby.GameRoom) error {
	room.Mu.Lock()
	n := len(room.Participants)
	if n < MinPlayers {
		room.Mu.Unlock()
		return fmt.Errorf("need at least %d players", MinPlayers)
	}
	if n > MaxPlayers {
		room.Mu.Unlock()
		return fmt.Errorf("maximum %d players", MaxPlayers)
	}

	players := make([]Player, 0, n)
	sessions := make(map[int64]*session.Session, n)
	for pid, sess := range room.Participants {
		players = append(players, Player{ID: pid, Username: sess.String("username"), Alive: true})
		sessions[pid] = sess
	}
	rand.Shuffle(len(players), func(i, j int) { players[i], players[j] = players[j], players[i] })

	config := defaultConfig()
	if room.Config != "" {
		if err := json.Unmarshal([]byte(room.Config), &config); err != nil {
			config = defaultConfig()
		}
	}
	if err := validateConfig(config, n); err != nil {
		config = defaultConfigForPlayers(n)
	}

	roles := distributeRoles(players, config)

	state := &GameState{
		Players:          players,
		DeadPlayers:       []Player{},
		Phase:            phaseNight,
		Round:            1,
		Config:           config,
		NightReadyRoles:  []string{},
		LastNightKills:   []Player{},
		VoteResults:      []VoteTally{},
		VoteCandidates:   []int64{},
		History:          []RoundLog{},
		Status:           "playing",
		HostID:           room.HostID,
		roles:            roles,
		nightActions:     make(map[int64]*NightActionData),
		detectiveLog:     []InvestigatePayload{},
		doctorLastSave:   -1,
		mafiaLastTarget:  -1,
		votes:            make(map[int64]int64),
	}

	state.PhaseEndsAt = time.Now().Add(time.Duration(config.NightTimerMs) * time.Millisecond).UnixMilli()
	state.LastEvent = "Night falls — roles have been assigned."
	addHistory(state, state.LastEvent)

	room.GameData = state
	roomID := room.ID

	state.onDisconnect = func(uid int64) {
		go func() {
			r, err := g.lobby.GetRoom(roomID)
			if err != nil {
				return
			}
			g.broadcast(r, state)
			if state.Phase == phaseGameOver {
				g.pushAllRoles(state, snapshotSessions(r))
			}
			g.lobby.PersistRoomAndState(r)
		}()
	}

	g.chat.RegisterRoomTabs(room.ID, []chat.ChatTabDef{
		{Name: "general", Label: "General", Visibility: "all", SendableBy: "all"},
		{Name: "mafia", Label: "Mafia", Visibility: "team:mafia", SendableBy: "team:mafia"},
	})

	room.Mu.Unlock()

	for _, p := range state.Players {
		if sess, ok := sessions[p.ID]; ok {
			payload := buildRolePayload(p.ID, state)
			sess.Push("onMafiaRole", payload)
		}
	}

	for _, p := range state.Players {
		if !p.Alive {
			continue
		}
		role := state.roles[p.ID]
		hasAction := role == roleMafia || role == roleGodfather ||
			role == roleDetective || role == roleDoctor ||
			(role == roleSheriff && !state.SheriffUsedAbility)
		if hasAction {
			targets := getNightTargets(state, p.ID, role)
			if sess, ok := sessions[p.ID]; ok {
				sess.Push("onMafiaNightTargets", map[string]interface{}{
					"targets": targets,
					"role":    role,
				})
			}
		}
	}

	g.broadcast(room, state)
	g.lobby.PersistRoomAndState(room)
	g.scheduleTimer(roomID, "night", config.NightTimerMs, func() {
		g.resolveNight(roomID)
	})
	g.scheduleSimulatedReadiness(roomID)
	return nil
}

func (g *Mafia) RestoreGame(room *lobby.GameRoom) {
	room.Mu.RLock()
	state, ok := room.GameData.(*GameState)
	if !ok {
		room.Mu.RUnlock()
		return
	}
	if state.Status == "finished" {
		room.Mu.RUnlock()
		return
	}
	roomID := room.ID
	phase := state.Phase
	remaining := state.PhaseEndsAt - time.Now().UnixMilli()
	if remaining < 3000 {
		remaining = 3000
	}
	room.Mu.RUnlock()

	switch phase {
	case phaseNight:
		g.scheduleTimer(roomID, "night", int(remaining), func() {
			g.resolveNight(roomID)
		})
	case phaseNightResult:
		g.scheduleTimer(roomID, "night_result", nightResultMs, func() {
			g.advanceFromNightResult(roomID)
		})
	case phaseDayDiscussion:
		g.scheduleTimer(roomID, "discussion", int(remaining), func() {
			g.advanceToVote(roomID)
		})
	case phaseDayVote:
		g.scheduleTimer(roomID, "vote", int(remaining), func() {
			g.resolveVote(roomID)
		})
	case phaseDayVoteResult:
		g.scheduleTimer(roomID, "vote_result", voteResultMs, func() {
			g.advanceFromVoteResult(roomID)
		})
	}
}

func (g *Mafia) GetState(s *session.Session, req *RoomReq) error {
	room, err := g.lobby.GetRoom(req.RoomID)
	if err != nil {
		return s.Response(&protocol.Response{Code: 404, Message: "room not found"})
	}
	uid := s.UID()

	room.Mu.RLock()
	state, ok := room.GameData.(*GameState)
	room.Mu.RUnlock()
	if !ok {
		return s.Response(&protocol.Response{Code: 404, Message: "no state"})
	}

	if err := s.Response(state); err != nil {
		return err
	}

	if uid != 0 && state.roles != nil && len(state.roles) > 0 {
		payload := buildRolePayload(uid, state)
		s.Push("onMafiaRole", payload)
		g.pushPrivateNightInfo(s, uid, state)
	}
	return nil
}

func (g *Mafia) Configure(s *session.Session, req *ConfigureReq) error {
	uid, _, err := auth.BindPlayer(s)
	if err != nil {
		return s.Response(&protocol.Response{Code: 401, Message: err.Error()})
	}
	room, err := g.lobby.GetRoom(req.RoomID)
	if err != nil {
		return s.Response(&protocol.Response{Code: 404, Message: "room not found"})
	}

	room.Mu.Lock()
	if room.HostID != uid {
		room.Mu.Unlock()
		return s.Response(&protocol.Response{Code: 403, Message: "only the host can configure"})
	}
	if room.Status != "waiting" && room.Status != "finished" {
		room.Mu.Unlock()
		return s.Response(&protocol.Response{Code: 400, Message: "game already started"})
	}

	config := req.Config
	n := len(room.Participants)
	if n < MinPlayers {
		n = MinPlayers
	}
	if err := validateConfig(config, n); err != nil {
		room.Mu.Unlock()
		return s.Response(&protocol.Response{Code: 400, Message: err.Error()})
	}

	configJSON, err := json.Marshal(config)
	if err != nil {
		room.Mu.Unlock()
		return s.Response(&protocol.Response{Code: 500, Message: "failed to marshal config"})
	}
	room.Config = string(configJSON)
	room.Mu.Unlock()

	db.UpdateRoomConfig(room.ID, room.Config)

	g.lobby.PersistRoomAndState(room)
	return s.Response(&protocol.Response{Code: 0})
}

func (g *Mafia) NightAction(s *session.Session, req *NightActionReq) error {
	uid, _, err := auth.BindPlayer(s)
	if err != nil {
		return s.Response(&protocol.Response{Code: 401, Message: err.Error()})
	}
	room, err := g.lobby.GetRoom(req.RoomID)
	if err != nil {
		return s.Response(&protocol.Response{Code: 404, Message: "room not found"})
	}

	room.Mu.Lock()
	state, ok := room.GameData.(*GameState)
	if !ok || state.Status != "playing" {
		room.Mu.Unlock()
		return s.Response(&protocol.Response{Code: 400, Message: "game not active"})
	}
	if state.Phase != phaseNight {
		room.Mu.Unlock()
		return s.Response(&protocol.Response{Code: 400, Message: "not night phase"})
	}

	player := findAlivePlayer(state.Players, uid)
	if player == nil {
		room.Mu.Unlock()
		return s.Response(&protocol.Response{Code: 403, Message: "you are not an active player"})
	}

	role := state.roles[uid]

	if _, exists := state.nightActions[uid]; exists {
		room.Mu.Unlock()
		return s.Response(&protocol.Response{Code: 400, Message: "already submitted night action"})
	}

	switch role {
	case roleMafia, roleGodfather:
		if req.Skip {
			room.Mu.Unlock()
			return s.Response(&protocol.Response{Code: 400, Message: "mafia must select a target"})
		}
		target := findAlivePlayer(state.Players, req.TargetID)
		if target == nil {
			room.Mu.Unlock()
			return s.Response(&protocol.Response{Code: 400, Message: "invalid target"})
		}
		if state.roles[req.TargetID] == roleMafia || state.roles[req.TargetID] == roleGodfather {
			room.Mu.Unlock()
			return s.Response(&protocol.Response{Code: 400, Message: "cannot target fellow mafia"})
		}
		if state.mafiaLastTarget != -1 && req.TargetID == state.mafiaLastTarget {
			room.Mu.Unlock()
			return s.Response(&protocol.Response{Code: 400, Message: "cannot target same player as last night"})
		}
		state.nightActions[uid] = &NightActionData{MafiaTarget: req.TargetID}
		state.MafiaReadyCount++

		state.NightReadyRoles = addUnique(state.NightReadyRoles, "Mafia")

		allDone := g.allNightActionsSubmitted(state)
		sessions := snapshotSessions(room)
		mafiaIDs := getPlayersWithRole(state, roleMafia, roleGodfather)
		markIDs := []int64{}
		for _, mid := range mafiaIDs {
			if na, ok := state.nightActions[mid]; ok && na.MafiaTarget != 0 {
				markIDs = append(markIDs, na.MafiaTarget)
			}
		}
		roomID := room.ID
		room.Mu.Unlock()
		g.broadcast(room, state)
		g.lobby.PersistRoomAndState(room)

		for _, mid := range mafiaIDs {
			if sess, ok := sessions[mid]; ok {
				sess.Push("onMafiaMarks", &MafiaMarkPayload{TargetIDs: markIDs})
			}
		}

		if allDone {
			g.cancelTimer(roomID)
			go g.resolveNight(roomID)
		}

	case roleDetective:
		target := findAlivePlayer(state.Players, req.TargetID)
		if target == nil || req.TargetID == uid {
			room.Mu.Unlock()
			return s.Response(&protocol.Response{Code: 400, Message: "invalid target"})
		}
		state.nightActions[uid] = &NightActionData{DetectiveTarget: req.TargetID}
		state.NightReadyRoles = addUnique(state.NightReadyRoles, "Detective")
		allDone := g.allNightActionsSubmitted(state)
		roomID := room.ID
		room.Mu.Unlock()
		g.broadcast(room, state)
		g.lobby.PersistRoomAndState(room)
		if allDone {
			g.cancelTimer(roomID)
			go g.resolveNight(roomID)
		}

	case roleDoctor:
		target := findAlivePlayer(state.Players, req.TargetID)
		if target == nil {
			room.Mu.Unlock()
			return s.Response(&protocol.Response{Code: 400, Message: "invalid target"})
		}
		if state.doctorLastSave != -1 && req.TargetID == state.doctorLastSave {
			room.Mu.Unlock()
			return s.Response(&protocol.Response{Code: 400, Message: "cannot protect the same player two nights in a row"})
		}
		state.nightActions[uid] = &NightActionData{DoctorTarget: req.TargetID}
		state.NightReadyRoles = addUnique(state.NightReadyRoles, "Doctor")
		allDone := g.allNightActionsSubmitted(state)
		roomID := room.ID
		room.Mu.Unlock()
		g.broadcast(room, state)
		g.lobby.PersistRoomAndState(room)
		if allDone {
			g.cancelTimer(roomID)
			go g.resolveNight(roomID)
		}

	case roleSheriff:
		if state.SheriffUsedAbility {
			room.Mu.Unlock()
			return s.Response(&protocol.Response{Code: 400, Message: "sheriff ability already used"})
		}
		if req.Skip {
			state.nightActions[uid] = &NightActionData{SheriffSkip: true}
			state.NightReadyRoles = addUnique(state.NightReadyRoles, "Sheriff")
			allDone := g.allNightActionsSubmitted(state)
			roomID := room.ID
			room.Mu.Unlock()
			g.broadcast(room, state)
			g.lobby.PersistRoomAndState(room)
			if allDone {
				g.cancelTimer(roomID)
				go g.resolveNight(roomID)
			}
		} else {
			target := findAlivePlayer(state.Players, req.TargetID)
			if target == nil || req.TargetID == uid {
				room.Mu.Unlock()
				return s.Response(&protocol.Response{Code: 400, Message: "invalid target"})
			}
			state.nightActions[uid] = &NightActionData{SheriffTarget: req.TargetID}
			state.NightReadyRoles = addUnique(state.NightReadyRoles, "Sheriff")
			allDone := g.allNightActionsSubmitted(state)
			roomID := room.ID
			room.Mu.Unlock()
			g.broadcast(room, state)
			g.lobby.PersistRoomAndState(room)
			if allDone {
				g.cancelTimer(roomID)
				go g.resolveNight(roomID)
			}
		}

	default:
		room.Mu.Unlock()
		return s.Response(&protocol.Response{Code: 400, Message: "no night action for your role"})
	}

	return s.Response(&protocol.Response{Code: 0})
}

func (g *Mafia) CastVote(s *session.Session, req *CastVoteReq) error {
	uid, _, err := auth.BindPlayer(s)
	if err != nil {
		return s.Response(&protocol.Response{Code: 401, Message: err.Error()})
	}
	room, err := g.lobby.GetRoom(req.RoomID)
	if err != nil {
		return s.Response(&protocol.Response{Code: 404, Message: "room not found"})
	}

	room.Mu.Lock()
	state, ok := room.GameData.(*GameState)
	if !ok || state.Status != "playing" {
		room.Mu.Unlock()
		return s.Response(&protocol.Response{Code: 400, Message: "game not active"})
	}
	if state.Phase != phaseDayVote {
		room.Mu.Unlock()
		return s.Response(&protocol.Response{Code: 400, Message: "not voting phase"})
	}

	if findAlivePlayer(state.Players, uid) == nil {
		room.Mu.Unlock()
		return s.Response(&protocol.Response{Code: 403, Message: "you are not an active player"})
	}
	if _, already := state.votes[uid]; already {
		room.Mu.Unlock()
		return s.Response(&protocol.Response{Code: 400, Message: "already voted"})
	}

	validTarget := false
	for _, cid := range state.VoteCandidates {
		if cid == req.TargetID {
			validTarget = true
			break
		}
	}
	if !validTarget {
		room.Mu.Unlock()
		return s.Response(&protocol.Response{Code: 400, Message: "invalid vote target"})
	}
	if req.TargetID == uid {
		room.Mu.Unlock()
		return s.Response(&protocol.Response{Code: 400, Message: "you cannot vote for yourself"})
	}

	state.votes[uid] = req.TargetID
	state.VotesIn++

	allIn := state.VotesIn >= countAlive(state)
	roomID := room.ID
	room.Mu.Unlock()
	g.broadcast(room, state)

	if allIn {
		g.cancelTimer(roomID)
		g.resolveVote(roomID)
	}

	return s.Response(&protocol.Response{Code: 0})
}

func (g *Mafia) EndDiscussion(s *session.Session, req *RoomReq) error {
	uid, _, err := auth.BindPlayer(s)
	if err != nil {
		return s.Response(&protocol.Response{Code: 401, Message: err.Error()})
	}
	room, err := g.lobby.GetRoom(req.RoomID)
	if err != nil {
		return s.Response(&protocol.Response{Code: 404, Message: "room not found"})
	}

	room.Mu.Lock()
	state, ok := room.GameData.(*GameState)
	if !ok || state.Status != "playing" {
		room.Mu.Unlock()
		return s.Response(&protocol.Response{Code: 400, Message: "game not active"})
	}
	if room.HostID != uid {
		room.Mu.Unlock()
		return s.Response(&protocol.Response{Code: 403, Message: "only the host can end discussion"})
	}
	if state.Phase != phaseDayDiscussion {
		room.Mu.Unlock()
		return s.Response(&protocol.Response{Code: 400, Message: "not in discussion phase"})
	}
	roomID := room.ID
	room.Mu.Unlock()
	g.cancelTimer(roomID)
	g.advanceToVote(roomID)
	return s.Response(&protocol.Response{Code: 0})
}

func (g *Mafia) SkipPhase(s *session.Session, req *RoomReq) error {
	uid, _, err := auth.BindPlayer(s)
	if err != nil {
		return s.Response(&protocol.Response{Code: 401, Message: err.Error()})
	}
	room, err := g.lobby.GetRoom(req.RoomID)
	if err != nil {
		return s.Response(&protocol.Response{Code: 404, Message: "room not found"})
	}

	room.Mu.Lock()
	state, ok := room.GameData.(*GameState)
	if !ok || state.Status != "playing" {
		room.Mu.Unlock()
		return s.Response(&protocol.Response{Code: 400, Message: "game not active"})
	}
	if room.HostID != uid {
		room.Mu.Unlock()
		return s.Response(&protocol.Response{Code: 403, Message: "only the host can skip phases"})
	}
	roomID := room.ID
	phase := state.Phase
	room.Mu.Unlock()

	g.cancelTimer(roomID)

	switch phase {
	case phaseNight:
		g.resolveNight(roomID)
	case phaseNightResult:
		g.advanceFromNightResult(roomID)
	case phaseDayDiscussion:
		g.advanceToVote(roomID)
	case phaseDayVote:
		g.resolveVote(roomID)
	case phaseDayVoteResult:
		g.advanceFromVoteResult(roomID)
	}

	return s.Response(&protocol.Response{Code: 0})
}

// ─── Night Resolution ──────────────────────────────────────────────────────

func (g *Mafia) resolveNight(roomID string) {
	room, err := g.lobby.GetRoom(roomID)
	if err != nil {
		return
	}
	room.Mu.Lock()
	state, ok := room.GameData.(*GameState)
	if !ok || state.Status != "playing" || state.Phase != phaseNight {
		room.Mu.Unlock()
		return
	}

	mafiaTargets := []int64{}
	for pid, na := range state.nightActions {
		if state.roles[pid] == roleMafia || state.roles[pid] == roleGodfather {
			if na != nil && na.MafiaTarget != 0 {
				mafiaTargets = append(mafiaTargets, na.MafiaTarget)
			}
		}
	}

	var sheriffAction *NightActionData
	var doctorAction *NightActionData
	var detectiveAction *NightActionData
	for pid, na := range state.nightActions {
		switch state.roles[pid] {
		case roleSheriff:
			if na != nil {
				sheriffAction = na
			}
		case roleDoctor:
			if na != nil {
				doctorAction = na
			}
		case roleDetective:
			if na != nil {
				detectiveAction = na
			}
		}
	}

	killed := []Player{}
	savedByDoctor := false
	var sheriffKillTarget *Player
	sherrifDied := false

	mafiaKill := resolveMafiaTarget(mafiaTargets, state)

	if sheriffAction != nil && !sheriffAction.SheriffSkip && sheriffAction.SheriffTarget != 0 {
		state.SheriffUsedAbility = true
		targetID := sheriffAction.SheriffTarget
		targetRole := state.roles[targetID]
		if targetRole == roleMafia || targetRole == roleGodfather {
			p := findAlivePlayer(state.Players, targetID)
			if p != nil {
				killed = append(killed, *p)
				pCopy := *p
				sheriffKillTarget = &pCopy
			}
		} else {
			sheriffID := findPlayerWithRole(state, roleSheriff)
			if sheriffID != 0 {
				p := findAlivePlayer(state.Players, sheriffID)
				if p != nil {
					killed = append(killed, *p)
					sherrifDied = true
				}
			}
		}
	}

	if mafiaKill != 0 {
		protectedByDoctor := false
		if doctorAction != nil && doctorAction.DoctorTarget == mafiaKill {
			protectedByDoctor = true
			savedByDoctor = true
		}
		if !protectedByDoctor {
			alreadyDead := false
			for _, k := range killed {
				if k.ID == mafiaKill {
					alreadyDead = true
					break
				}
			}
			if !alreadyDead {
				p := findAlivePlayer(state.Players, mafiaKill)
				if p != nil {
					killed = append(killed, *p)
				}
			}
		}
	}

	if detectiveAction != nil && detectiveAction.DetectiveTarget != 0 {
		targetID := detectiveAction.DetectiveTarget
		targetRole := state.roles[targetID]
		resultStr := "civilian"
		if targetRole == roleMafia {
			resultStr = "mafia"
		}
		if targetRole == roleGodfather {
			resultStr = "civilian"
		}

		targetPlayer := findAlivePlayer(state.Players, targetID)
		targetName := "Unknown"
		if targetPlayer != nil {
			targetName = targetPlayer.Username
		}
		invResult := InvestigatePayload{
			TargetID:       targetID,
			TargetUsername: targetName,
			Result:         resultStr,
		}
		state.detectiveLog = append(state.detectiveLog, invResult)

		sessions := snapshotSessions(room)
		detID := findPlayerWithRole(state, roleDetective)
		if detID != 0 {
			if sess, ok := sessions[detID]; ok {
				if findAlivePlayer(state.Players, detID) != nil {
					sess.Push("onMafiaInvestigate", invResult)
				}
			}
		}
	}

	state.doctorLastSave = -1
	if doctorAction != nil && doctorAction.DoctorTarget != 0 {
		state.doctorLastSave = doctorAction.DoctorTarget
	}

	state.mafiaLastTarget = mafiaKill

	for i := range killed {
		for j := range state.Players {
			if state.Players[j].ID == killed[i].ID {
				state.Players[j].Alive = false
				break
			}
		}
		deadP := killed[i]
		deadP.Role = state.roles[deadP.ID]
		state.DeadPlayers = append(state.DeadPlayers, deadP)
		state.Players = removePlayerFromAlive(state.Players, deadP.ID)
	}

	for i := range killed {
		killed[i].Role = state.roles[killed[i].ID]
	}

	state.LastNightKills = killed
	state.LastNightSaved = savedByDoctor

	if len(killed) > 0 {
		for _, k := range killed {
			addHistoryWithTarget(state, k.ID, k.Username, fmt.Sprintf("%s was eliminated at night (role: %s)", k.Username, k.Role))
		}
	} else if savedByDoctor {
		addHistory(state, "Doctor saved someone during the night")
	} else {
		addHistory(state, "Night passed quietly, no eliminations")
	}
	if sherrifDied {
		addHistory(state, "The Sheriff misfired and died!")
	}
	if sheriffKillTarget != nil {
		addHistoryWithTarget(state, sheriffKillTarget.ID, sheriffKillTarget.Username, fmt.Sprintf("Sheriff killed %s (was Mafia)", sheriffKillTarget.Username))
	}

	sessions := snapshotSessions(room)

	if won, winner, reason := checkWin(state); won {
		state.Status = "finished"
		state.Phase = phaseGameOver
		state.Winner = winner
		state.WinReason = reason
		state.LastEvent = formatWinMessage(winner, reason)
		state.PhaseEndsAt = 0
		state.nightActions = make(map[int64]*NightActionData)
		state.NightReadyRoles = []string{}
		addHistory(state, fmt.Sprintf("Game Over — %s", state.LastEvent))

		g.pushNightResults(state, sessions, killed, savedByDoctor, sherrifDied, sheriffKillTarget)

		room.Mu.Unlock()
		g.broadcast(room, state)
		room.Mu.Lock()
		room.Status = "finished"
		room.Mu.Unlock()
		g.lobby.PersistRoomAndState(room)
		g.pushAllRoles(state, sessions)
		return
	}

	state.Phase = phaseNightResult
	var savedPlayerName string
	if savedByDoctor && doctorAction != nil && doctorAction.DoctorTarget != 0 {
		if p := findAlivePlayer(state.Players, doctorAction.DoctorTarget); p != nil {
			savedPlayerName = p.Username
		}
	}
	state.LastEvent = formatNightResult(killed, savedByDoctor, sherrifDied, sheriffKillTarget, state.Config.AnnounceSavedPlayer, savedPlayerName)
	state.LastNightEvent = state.LastEvent
	state.PhaseEndsAt = time.Now().Add(nightResultMs * time.Millisecond).UnixMilli()
	state.nightActions = make(map[int64]*NightActionData)
	state.NightReadyRoles = []string{}

	g.pushNightResults(state, sessions, killed, savedByDoctor, sherrifDied, sheriffKillTarget)

	room.Mu.Unlock()
	g.broadcast(room, state)
	g.lobby.PersistRoomAndState(room)
	g.scheduleTimer(roomID, "night_result", nightResultMs, func() {
		g.advanceFromNightResult(roomID)
	})
}

func (g *Mafia) pushNightResults(state *GameState, sessions map[int64]*session.Session, killed []Player, savedByDoctor bool, sheriffDied bool, sheriffKillTarget *Player) {
	result := &NightResultPayload{
		KilledPlayers: killed,
		NoKill:         len(killed) == 0 && !savedByDoctor,
		SheriffDied:    sheriffDied,
		SheriffKilled:  sheriffKillTarget,
		SavedByDoctor:  savedByDoctor,
	}
	for _, sess := range sessions {
		sess.Push("onMafiaNightResult", result)
	}
}

func (g *Mafia) pushAllRoles(state *GameState, sessions map[int64]*session.Session) {
	for _, p := range state.Players {
		if sess, ok := sessions[p.ID]; ok {
			sess.Push("onMafiaRole", &RolePayload{Role: state.roles[p.ID]})
		}
	}
	for _, p := range state.DeadPlayers {
		if sess, ok := sessions[p.ID]; ok {
			sess.Push("onMafiaRole", &RolePayload{Role: state.roles[p.ID]})
		}
	}
}

func (g *Mafia) advanceFromNightResult(roomID string) {
	room, err := g.lobby.GetRoom(roomID)
	if err != nil {
		return
	}
	room.Mu.Lock()
	state, ok := room.GameData.(*GameState)
	if !ok || state.Status != "playing" || state.Phase != phaseNightResult {
		room.Mu.Unlock()
		return
	}

	state.Phase = phaseDayDiscussion
	state.LastEvent = "Day breaks — discuss who you suspect."
	addHistory(state, state.LastEvent)
	state.PhaseEndsAt = time.Now().Add(time.Duration(state.Config.DiscussionTimerMs) * time.Millisecond).UnixMilli()
	state.VotesIn = 0
	state.TotalVoters = countAlive(state)
	state.VoteResults = nil
	state.IsRevote = false
	state.VoteCandidates = nil
	state.votes = make(map[int64]int64)

	room.Mu.Unlock()
	g.broadcast(room, state)
	g.lobby.PersistRoomAndState(room)
	g.scheduleTimer(roomID, "discussion", state.Config.DiscussionTimerMs, func() {
		g.advanceToVote(roomID)
	})
}

func (g *Mafia) advanceToVote(roomID string) {
	room, err := g.lobby.GetRoom(roomID)
	if err != nil {
		return
	}
	room.Mu.Lock()
	state, ok := room.GameData.(*GameState)
	if !ok || state.Status != "playing" {
		room.Mu.Unlock()
		return
	}
	if state.Phase != phaseDayDiscussion && state.Phase != phaseDayVote {
		room.Mu.Unlock()
		return
	}

	candidates := []int64{}
	for _, p := range state.Players {
		if p.Alive {
			candidates = append(candidates, p.ID)
		}
	}
	state.VoteCandidates = candidates

	if !state.IsRevote {
		state.votes = make(map[int64]int64)
		state.VotesIn = 0
		state.TotalVoters = countAlive(state)
		state.VoteResults = nil
	}

	state.Phase = phaseDayVote
	state.LastEvent = "Vote — select who to eliminate."
	addHistory(state, state.LastEvent)
	state.PhaseEndsAt = time.Now().Add(time.Duration(state.Config.VoteTimerMs) * time.Millisecond).UnixMilli()

	roomID2 := room.ID
	room.Mu.Unlock()
	g.broadcast(room, state)
	g.lobby.PersistRoomAndState(room)
	g.scheduleTimer(roomID2, "vote", state.Config.VoteTimerMs, func() {
		g.resolveVote(roomID2)
	})
}

func (g *Mafia) resolveVote(roomID string) {
	room, err := g.lobby.GetRoom(roomID)
	if err != nil {
		return
	}
	room.Mu.Lock()
	state, ok := room.GameData.(*GameState)
	if !ok || state.Status != "playing" || state.Phase != phaseDayVote {
		room.Mu.Unlock()
		return
	}

	tally := make(map[int64]int)
	for _, targetID := range state.VoteCandidates {
		tally[targetID] = 0
	}
	for _, targetID := range state.votes {
		if _, valid := tally[targetID]; valid {
			tally[targetID]++
		}
	}

	var sorted []VoteTally
	for _, p := range state.Players {
		if p.Alive {
			if count, ok := tally[p.ID]; ok && count > 0 {
				sorted = append(sorted, VoteTally{PlayerID: p.ID, Username: p.Username, VoteCount: count})
			}
		}
	}
	sort.Slice(sorted, func(i, j int) bool { return sorted[i].VoteCount > sorted[j].VoteCount })

	state.VoteResults = sorted
	state.Phase = phaseDayVoteResult
	state.PhaseEndsAt = time.Now().Add(voteResultMs * time.Millisecond).UnixMilli()

	if won, winner, reason := checkWinVote(state, sorted); won {
		jesterID := sorted[0].PlayerID
		jesterRole := state.roles[jesterID]
		for i := range state.Players {
			if state.Players[i].ID == jesterID {
				state.Players[i].Alive = false
				break
			}
		}
		state.Players = removePlayerFromAlive(state.Players, jesterID)
		deadP := Player{ID: jesterID, Username: sorted[0].Username, Alive: false, Role: jesterRole}
		state.DeadPlayers = append(state.DeadPlayers, deadP)

		state.Status = "finished"
		state.Phase = phaseGameOver
		state.Winner = winner
		state.WinReason = reason
		state.LastEvent = formatWinMessage(winner, reason)
		state.PhaseEndsAt = 0

		addHistoryWithTarget(state, jesterID, sorted[0].Username, fmt.Sprintf("%s (Jester) was voted out and wins!", sorted[0].Username))

		sessions := snapshotSessions(room)
		g.pushAllRoles(state, sessions)

		room.Mu.Unlock()
		g.broadcast(room, state)
		room.Mu.Lock()
		room.Status = "finished"
		room.Mu.Unlock()
		g.lobby.PersistRoomAndState(room)
		return
	}

	maxVotes := 0
	if len(sorted) > 0 {
		maxVotes = sorted[0].VoteCount
	}
	var tied []VoteTally
	for _, v := range sorted {
		if v.VoteCount == maxVotes && maxVotes > 0 {
			tied = append(tied, v)
		}
	}

	if len(tied) > 1 {
		if state.IsRevote {
			state.LastEvent = "Vote tied again — no elimination. Night begins."
			addHistory(state, fmt.Sprintf("Revote tied again — no elimination (Round %d)", state.Round))
			// no elimination, go to night
			state.IsRevote = false
			state.VoteCandidates = nil
		} else {
			state.LastEvent = "Vote tied! Revote between tied players."
			addHistory(state, fmt.Sprintf("Vote tied between %v — revote initiated", tied))
			state.IsRevote = true
			candidates := []int64{}
			for _, t := range tied {
				candidates = append(candidates, t.PlayerID)
			}
			state.VoteCandidates = candidates
		}
	} else if len(tied) == 1 && maxVotes > 0 {
		eliminated := tied[0]
		elimRole := state.roles[eliminated.PlayerID]
		eliminated.Role = elimRole
		for i := range state.Players {
			if state.Players[i].ID == eliminated.PlayerID {
				state.Players[i].Alive = false
				break
			}
		}
		state.Players = removePlayerFromAlive(state.Players, eliminated.PlayerID)
		deadP := Player{ID: eliminated.PlayerID, Username: eliminated.Username, Alive: false, Role: elimRole}
		state.DeadPlayers = append(state.DeadPlayers, deadP)
		state.LastEvent = fmt.Sprintf("💀 %s was eliminated! They were %s %s.", eliminated.Username, roleEmoji(elimRole), roleLabel(elimRole))
		addHistoryWithTarget(state, eliminated.PlayerID, eliminated.Username, fmt.Sprintf("%s was voted out (role: %s)", eliminated.Username, elimRole))

		if won, winner, reason := checkWin(state); won {
			state.Status = "finished"
			state.Phase = phaseGameOver
			state.Winner = winner
			state.WinReason = reason
			state.LastEvent = formatWinMessage(winner, reason)
			state.PhaseEndsAt = 0
			addHistory(state, fmt.Sprintf("Game Over — %s", state.LastEvent))

			sessions := snapshotSessions(room)
			g.pushAllRoles(state, sessions)

			room.Mu.Unlock()
			g.broadcast(room, state)
			room.Mu.Lock()
			room.Status = "finished"
			room.Mu.Unlock()
			g.lobby.PersistRoomAndState(room)
			return
		}
	} else {
		state.LastEvent = "No votes cast — no elimination."
		addHistory(state, "No votes cast — no elimination")
	}

	roomID2 := room.ID
	room.Mu.Unlock()
	g.lobby.PersistRoomAndState(room)
	g.broadcast(room, state)
	g.scheduleTimer(roomID2, "vote_result", voteResultMs, func() {
		g.advanceFromVoteResult(roomID2)
	})
}

func (g *Mafia) advanceFromVoteResult(roomID string) {
	room, err := g.lobby.GetRoom(roomID)
	if err != nil {
		return
	}
	room.Mu.Lock()
	state, ok := room.GameData.(*GameState)
	if !ok || state.Status != "playing" {
		room.Mu.Unlock()
		return
	}
	if state.Phase != phaseDayVoteResult {
		room.Mu.Unlock()
		return
	}

	if state.IsRevote && state.VoteCandidates != nil && len(state.VoteCandidates) > 0 {
		state.IsRevote = true
		state.votes = make(map[int64]int64)
		state.VotesIn = 0
		state.TotalVoters = countAlive(state)
		state.VoteResults = nil
		state.Phase = phaseDayVote
		state.LastEvent = "Revote — select who to eliminate from the tied players."
		state.PhaseEndsAt = time.Now().Add(time.Duration(state.Config.VoteTimerMs) * time.Millisecond).UnixMilli()

		roomID2 := room.ID
		room.Mu.Unlock()
		g.broadcast(room, state)
		g.lobby.PersistRoomAndState(room)
		g.scheduleTimer(roomID2, "vote", state.Config.VoteTimerMs, func() {
			g.resolveVote(roomID2)
		})
		return
	}

	g.startNight(state, room)
	roomID2 := room.ID
	room.Mu.Unlock()
	g.broadcast(room, state)
	g.lobby.PersistRoomAndState(room)
	g.scheduleSimulatedReadiness(roomID2)
	g.scheduleTimer(roomID2, "night", state.Config.NightTimerMs, func() {
		g.resolveNight(roomID2)
	})
}

func (g *Mafia) startNight(state *GameState, room *lobby.GameRoom) {
	state.Round++
	state.Phase = phaseNight
	state.nightActions = make(map[int64]*NightActionData)
	state.NightReadyRoles = []string{}
	state.MafiaReadyCount = 0
	state.LastNightKills = []Player{}
	state.LastNightSaved = false
	state.LastNightEvent = ""
	state.VotesIn = 0
	state.TotalVoters = 0
	state.VoteResults = nil
	state.IsRevote = false
	state.VoteCandidates = nil
	state.votes = make(map[int64]int64)
	state.PhaseEndsAt = time.Now().Add(time.Duration(state.Config.NightTimerMs) * time.Millisecond).UnixMilli()
	state.LastEvent = fmt.Sprintf("Night %d falls — submit your actions.", state.Round)
	addHistory(state, state.LastEvent)

	sessions := snapshotSessions(room)
	for _, p := range state.Players {
		if !p.Alive {
			continue
		}
		role := state.roles[p.ID]
		hasAction := role == roleMafia || role == roleGodfather ||
			role == roleDetective || role == roleDoctor ||
			(role == roleSheriff && !state.SheriffUsedAbility)
		if hasAction {
			targets := getNightTargets(state, p.ID, role)
			if sess, ok := sessions[p.ID]; ok {
					sess.Push("onMafiaNightTargets", map[string]interface{}{
						"targets": targets,
						"role":    role,
					})
				}
			}
		}
}

func getNightTargets(state *GameState, playerID int64, role string) []Player {
	targets := []Player{}
	switch role {
	case roleMafia, roleGodfather:
		for _, p := range state.Players {
			if !p.Alive {
				continue
			}
			if p.ID == playerID {
				continue
			}
			pRole := state.roles[p.ID]
			if pRole == roleMafia || pRole == roleGodfather {
				continue
			}
			if state.mafiaLastTarget != -1 && p.ID == state.mafiaLastTarget {
				continue
			}
			targets = append(targets, p)
		}
	case roleDetective:
		for _, p := range state.Players {
			if !p.Alive {
				continue
			}
			if p.ID == playerID {
				continue
			}
			targets = append(targets, p)
		}
	case roleDoctor:
		for _, p := range state.Players {
			if !p.Alive {
				continue
			}
			if state.doctorLastSave != -1 && p.ID == state.doctorLastSave {
				continue
			}
			targets = append(targets, p)
		}
	case roleSheriff:
		for _, p := range state.Players {
			if !p.Alive {
				continue
			}
			if p.ID == playerID {
				continue
			}
			targets = append(targets, p)
		}
	}
	return targets
}

// ─── Game Logic Helpers ─────────────────────────────────────────────────────

func (g *Mafia) allNightActionsSubmitted(state *GameState) bool {
	for _, p := range state.Players {
		if !p.Alive {
			continue
		}
		role := state.roles[p.ID]
		needsAction := false
		switch role {
		case roleMafia, roleGodfather:
			needsAction = true
		case roleDetective:
			if state.Config.HasDetective {
				needsAction = true
			}
		case roleDoctor:
			if state.Config.HasDoctor {
				needsAction = true
			}
		case roleSheriff:
			if state.Config.HasSheriff && !state.SheriffUsedAbility {
				needsAction = false
			}
		}
		if needsAction {
			if _, ok := state.nightActions[p.ID]; !ok {
				return false
			}
		}
	}
	return true
}

func resolveMafiaTarget(targets []int64, state *GameState) int64 {
	if len(targets) == 0 {
		return 0
	}
	if len(targets) == 1 {
		return targets[0]
	}
	counts := make(map[int64]int)
	for _, t := range targets {
		counts[t]++
	}
	topCount := 0
	for _, c := range counts {
		if c > topCount {
			topCount = c
		}
	}
	numDisagree := len(targets) - topCount
	if numDisagree <= 2 {
		return targets[rand.Intn(len(targets))]
	}
	var maxTargets []int64
	for t, c := range counts {
		if c == topCount {
			maxTargets = append(maxTargets, t)
		}
	}
	return maxTargets[rand.Intn(len(maxTargets))]
}

func checkWin(state *GameState) (bool, string, string) {
	aliveMafia := 0
	aliveCivilians := 0
	for _, p := range state.Players {
		if !p.Alive {
			continue
		}
		role := state.roles[p.ID]
		if role == roleMafia || role == roleGodfather {
			aliveMafia++
		} else if role == roleJester {
			// Neutral — don't count for either side
		} else {
			aliveCivilians++
		}
	}
	if aliveMafia == 0 {
		return true, winCivilian, "All Mafia have been eliminated!"
	}
	if aliveMafia >= aliveCivilians {
		return true, winMafia, "Mafia outnumber the Civilians!"
	}
	return false, "", ""
}

func checkWinVote(state *GameState, sorted []VoteTally) (bool, string, string) {
	if len(sorted) == 0 {
		return false, "", ""
	}
	maxVotes := sorted[0].VoteCount
	if maxVotes == 0 {
		return false, "", ""
	}
	var tied []VoteTally
	for _, v := range sorted {
		if v.VoteCount == maxVotes {
			tied = append(tied, v)
		}
	}
	if len(tied) != 1 {
		return false, "", ""
	}
	elimID := tied[0].PlayerID
	elimRole := state.roles[elimID]

	if elimRole == roleJester {
		state.JesterWon = true
		return true, winJester, fmt.Sprintf("The Jester was voted out! %s wins!", roleLabel(roleJester))
	}
	return false, "", ""
}

func distributeRoles(players []Player, config GameConfig) map[int64]string {
	n := len(players)
	roles := make(map[int64]string, n)

	indices := rand.Perm(n)
	ptr := 0

	numMafia := config.NumMafia
	if config.HasGodfather && numMafia > 0 {
		roles[players[indices[ptr]].ID] = roleGodfather
		ptr++
		numMafia--
	}
	for i := 0; i < numMafia; i++ {
		roles[players[indices[ptr]].ID] = roleMafia
		ptr++
	}

	if config.HasDetective && ptr < n {
		roles[players[indices[ptr]].ID] = roleDetective
		ptr++
	}
	if config.HasDoctor && ptr < n {
		roles[players[indices[ptr]].ID] = roleDoctor
		ptr++
	}
	if config.HasSheriff && ptr < n {
		roles[players[indices[ptr]].ID] = roleSheriff
		ptr++
	}
	if config.HasJester && ptr < n {
		roles[players[indices[ptr]].ID] = roleJester
		ptr++
	}

	for ; ptr < n; ptr++ {
		roles[players[indices[ptr]].ID] = roleCivilian
	}
	return roles
}

func buildRolePayload(playerID int64, state *GameState) *RolePayload {
	role := state.roles[playerID]
	teammates := []Player{}

	if role == roleMafia || role == roleGodfather {
		for _, p := range state.Players {
			if p.ID != playerID {
				r := state.roles[p.ID]
				if r == roleMafia || r == roleGodfather {
					teammates = append(teammates, p)
				}
			}
		}
		for _, p := range state.DeadPlayers {
			r := state.roles[p.ID]
			if r == roleMafia || r == roleGodfather {
				teammates = append(teammates, p)
			}
		}
	}

	return &RolePayload{Role: role, KnownTeammates: teammates}
}

func validateConfig(config GameConfig, playerCount int) error {
	maxMafia := maxMafiaForPlayers(playerCount)
	if config.NumMafia < 1 || config.NumMafia > maxMafia {
		return fmt.Errorf("mafia count must be between 1 and %d for %d players", maxMafia, playerCount)
	}
	if config.HasGodfather && config.NumMafia < 2 {
		return fmt.Errorf("godfather requires at least 2 mafia members")
	}
	if config.HasGodfather && !config.HasDetective {
		return fmt.Errorf("godfather requires detective to be enabled")
	}
	specialCount := 0
	if config.HasDetective {
		specialCount++
	}
	if config.HasDoctor {
		specialCount++
	}
	if config.HasSheriff {
		specialCount++
	}
	if config.HasJester {
		specialCount++
	}
	if config.NumMafia+specialCount > playerCount {
		return fmt.Errorf("not enough players for this configuration")
	}
	return nil
}

func defaultConfigForPlayers(n int) GameConfig {
	config := defaultConfig()
	config.NumMafia = 1
	if n >= 8 {
		config.NumMafia = 2
	}
	return config
}

func maxMafiaForPlayers(n int) int {
	switch {
	case n <= 5:
		return 1
	case n <= 7:
		return 2
	case n <= 9:
		return 3
	default:
		return 4
	}
}

func findAlivePlayer(players []Player, id int64) *Player {
	for i := range players {
		if players[i].ID == id && players[i].Alive {
			return &players[i]
		}
	}
	return nil
}

func findPlayerWithRole(state *GameState, role string) int64 {
	for _, p := range state.Players {
		if state.roles[p.ID] == role && p.Alive {
			return p.ID
		}
	}
	return 0
}

func getPlayersWithRole(state *GameState, roles ...string) []int64 {
	ids := []int64{}
	for _, p := range state.Players {
		if !p.Alive {
			continue
		}
		for _, r := range roles {
			if state.roles[p.ID] == r {
				ids = append(ids, p.ID)
				break
			}
		}
	}
	return ids
}

func removePlayerFromAlive(players []Player, id int64) []Player {
	for i, p := range players {
		if p.ID == id {
			return append(players[:i], players[i+1:]...)
		}
	}
	return players
}

func countAlive(state *GameState) int {
	count := 0
	for _, p := range state.Players {
		if p.Alive {
			count++
		}
	}
	return count
}

func addUnique(slice []string, val string) []string {
	for _, v := range slice {
		if v == val {
			return slice
		}
	}
	return append(slice, val)
}

func containsString(slice []string, val string) bool {
	for _, v := range slice {
		if v == val {
			return true
		}
	}
	return false
}

func roleEmoji(role string) string {
	switch role {
	case roleMafia:
		return "🔴"
	case roleCivilian:
		return "⚪"
	case roleDetective:
		return "🔵"
	case roleDoctor:
		return "🟢"
	case roleSheriff:
		return "🌟"
	case roleGodfather:
		return "⚫"
	case roleJester:
		return "🟠"
	default:
		return "⚪"
	}
}

func roleLabel(role string) string {
	switch role {
	case roleMafia:
		return "Mafia"
	case roleCivilian:
		return "Civilian"
	case roleDetective:
		return "Detective"
	case roleDoctor:
		return "Doctor"
	case roleSheriff:
		return "Sheriff"
	case roleGodfather:
		return "Godfather"
	case roleJester:
		return "Jester"
	default:
		return role
	}
}

func formatNightResult(killed []Player, saved bool, sheriffDied bool, sheriffKill *Player, announceSaved bool, savedPlayerName string) string {
	parts := []string{}
	savedMsg := "Doctor successfully saved the target"
	if saved && announceSaved && savedPlayerName != "" {
		savedMsg = fmt.Sprintf("%s was saved by a doctor", savedPlayerName)
	}
	if len(killed) == 0 {
		if saved {
			parts = append(parts, savedMsg)
		} else {
			parts = append(parts, "The night passes quietly. No one was eliminated.")
		}
	} else {
		names := []string{}
		for _, p := range killed {
			names = append(names, p.Username)
		}
		parts = append(parts, fmt.Sprintf("Eliminated: %s", joinNames(names)))
	}
	if sheriffDied {
		parts = append(parts, "The Sheriff misfired and died!")
	}
	if sheriffKill != nil {
		parts = append(parts, fmt.Sprintf("The Sheriff shot %s — they were Mafia!", sheriffKill.Username))
	}
	if saved && len(killed) > 0 {
		parts = append(parts, savedMsg)
	}
	return joinWith(parts, ". ")
}

var roleRevealPatterns = []*regexp.Regexp{
	regexp.MustCompile(` ?\(role: [^)]+\)`),
	regexp.MustCompile(` ?— they were [^.!]+`),
	regexp.MustCompile(`! They were [^.!]+\.`),
}

func stripRoleFromEvent(s string) string {
	for _, pat := range roleRevealPatterns {
		s = pat.ReplaceAllString(s, "")
	}
	return s
}

func formatWinMessage(winner, reason string) string {
	switch winner {
	case winMafia:
		return "🔴 Mafia wins! " + reason
	case winCivilian:
		return "⚪ Civilians win! " + reason
	case winJester:
		return "🟠 Jester wins! " + reason
	default:
		return reason
	}
}

func joinNames(names []string) string {
	if len(names) == 1 {
		return names[0]
	}
	if len(names) == 2 {
		return names[0] + " and " + names[1]
	}
	result := ""
	for i, n := range names {
		if i == len(names)-1 {
			result += "and " + n
		} else {
			result += n + ", "
		}
	}
	return result
}

func addHistory(state *GameState, detail string) {
	state.History = append(state.History, RoundLog{
		Round:  state.Round,
		Phase:  state.Phase,
		Detail: detail,
	})
}

func addHistoryWithTarget(state *GameState, targetID int64, targetName string, detail string) {
	state.History = append(state.History, RoundLog{
		Round:      state.Round,
		Phase:      state.Phase,
		TargetID:   targetID,
		TargetName: targetName,
		Detail:     detail,
	})
}

func joinWith(parts []string, sep string) string {
	result := ""
	for i, p := range parts {
		if i > 0 {
			result += sep
		}
		result += p
	}
	return result
}

func (g *Mafia) pushPrivateNightInfo(s *session.Session, uid int64, state *GameState) {
	if state.Phase != phaseNight {
		return
	}
	if _, hasAction := state.nightActions[uid]; hasAction {
		s.Push("onMafiaActionDone", map[string]interface{}{"done": true})
		return
	}
	role := state.roles[uid]
	targets := getNightTargets(state, uid, role)
	if len(targets) == 0 {
		return
	}
	switch role {
	case roleMafia, roleGodfather, roleDetective, roleDoctor, roleSheriff:
		s.Push("onMafiaNightTargets", map[string]interface{}{
			"targets": targets,
			"role":    role,
		})
		if role == roleMafia || role == roleGodfather {
			mafiaIDs := getPlayersWithRole(state, roleMafia, roleGodfather)
			markIDs := []int64{}
			for _, mid := range mafiaIDs {
				if na, ok := state.nightActions[mid]; ok && na != nil && na.MafiaTarget != 0 {
					markIDs = append(markIDs, na.MafiaTarget)
				}
			}
			s.Push("onMafiaMarks", &MafiaMarkPayload{TargetIDs: markIDs})
		}
	}
}

// ─── Broadcast & Timers ──────────────────────────────────────────────────────

func (g *Mafia) broadcast(room *lobby.GameRoom, state *GameState) {
	// Perform a full deep-copy of the GameState so the serialized JSON
	// sent to clients never shares backing arrays with the live state,
	// preventing data races and potential circular-reference issues.
	deepCopy := func(src *GameState) *GameState {
		dst := &GameState{}
		*dst = *src

		// Deep-copy all slices
		dst.Players = make([]Player, len(src.Players))
		copy(dst.Players, src.Players)

		dst.DeadPlayers = make([]Player, len(src.DeadPlayers))
		copy(dst.DeadPlayers, src.DeadPlayers)

		dst.LastNightKills = make([]Player, len(src.LastNightKills))
		copy(dst.LastNightKills, src.LastNightKills)

		dst.NightReadyRoles = make([]string, len(src.NightReadyRoles))
		copy(dst.NightReadyRoles, src.NightReadyRoles)

		dst.VoteResults = make([]VoteTally, len(src.VoteResults))
		copy(dst.VoteResults, src.VoteResults)

		dst.VoteCandidates = make([]int64, len(src.VoteCandidates))
		copy(dst.VoteCandidates, src.VoteCandidates)

		dst.History = make([]RoundLog, len(src.History))
		copy(dst.History, src.History)

		if src.detectiveLog != nil {
			dst.detectiveLog = make([]InvestigatePayload, len(src.detectiveLog))
			copy(dst.detectiveLog, src.detectiveLog)
		}

		// Deep-copy maps
		if src.roles != nil {
			dst.roles = make(map[int64]string, len(src.roles))
			for k, v := range src.roles {
				dst.roles[k] = v
			}
		}
		if src.nightActions != nil {
			dst.nightActions = make(map[int64]*NightActionData, len(src.nightActions))
			for k, v := range src.nightActions {
				if v != nil {
					vc := *v
					dst.nightActions[k] = &vc
				}
			}
		}
		if src.votes != nil {
			dst.votes = make(map[int64]int64, len(src.votes))
			for k, v := range src.votes {
				dst.votes[k] = v
			}
		}

		return dst
	}

	var broadcastState *GameState

	if state.Phase == phaseGameOver {
		// Game over: reveal all roles, show team counts
		broadcastState = deepCopy(state)
		for i := range broadcastState.Players {
			broadcastState.Players[i].Role = state.roles[broadcastState.Players[i].ID]
		}
		for i := range broadcastState.History {
			broadcastState.History[i].Detail = stripRoleFromEvent(broadcastState.History[i].Detail)
		}
		broadcastState.TeamCounts = buildTeamCounts(state)
	} else if state.Config.RevealOnDeath {
		broadcastState = deepCopy(state)
		broadcastState.TeamCounts = buildTeamCounts(state)
	} else {
		// Hide roles from dead players and night-kill list
		broadcastState = deepCopy(state)
		for i := range broadcastState.DeadPlayers {
			broadcastState.DeadPlayers[i].Role = ""
		}
		for i := range broadcastState.LastNightKills {
			broadcastState.LastNightKills[i].Role = ""
		}
		broadcastState.LastEvent = stripRoleFromEvent(state.LastEvent)
		for i := range broadcastState.History {
			broadcastState.History[i].Detail = stripRoleFromEvent(broadcastState.History[i].Detail)
		}
		broadcastState.TeamCounts = nil
	}

	room.Mu.RLock()
	for _, sess := range room.Participants {
		s := sess
		go func() {
			defer func() { recover() }()
			s.Push("onMafiaUpdate", broadcastState)
		}()
	}
	for _, sess := range room.Spectators {
		s := sess
		go func() {
			defer func() { recover() }()
			s.Push("onMafiaUpdate", broadcastState)
		}()
	}
	room.Mu.RUnlock()
}

func buildTeamCounts(state *GameState) *TeamCounts {
	tc := &TeamCounts{}
	for _, p := range state.Players {
		role := state.roles[p.ID]
		switch role {
		case roleMafia, roleGodfather:
			tc.MafiaAlive++
		case roleJester:
			tc.NeutralAlive++
		default:
			tc.CivAlive++
		}
	}
	tc.TotalDead = len(state.DeadPlayers)
	return tc
}

func (g *Mafia) scheduleTimer(roomID, key string, delayMs int, fn func()) {
	fullKey := roomID + ":" + key
	g.mu.Lock()
	if t, ok := g.timers[fullKey]; ok {
		t.Stop()
	}
	g.timers[fullKey] = time.AfterFunc(time.Duration(delayMs)*time.Millisecond, fn)
	g.mu.Unlock()
}

func (g *Mafia) cancelTimer(roomID string) {
	g.mu.Lock()
	toCancel := []string{}
	for k := range g.timers {
		if len(k) > len(roomID) && k[:len(roomID)] == roomID {
			toCancel = append(toCancel, k)
		}
	}
	for _, k := range toCancel {
		g.timers[k].Stop()
		delete(g.timers, k)
	}
	g.mu.Unlock()
}

func (g *Mafia) scheduleSimulatedReadiness(roomID string) {
	room, err := g.lobby.GetRoom(roomID)
	if err != nil {
		return
	}
	room.Mu.RLock()
	state, ok := room.GameData.(*GameState)
	if !ok || state.Phase != phaseNight || state.Config.RevealOnDeath {
		room.Mu.RUnlock()
		return
	}

	aliveRoles := make(map[string]bool)
	for _, p := range state.Players {
		if p.Alive {
			aliveRoles[state.roles[p.ID]] = true
		}
	}

	simRoles := []string{}
	if state.Config.HasDetective && !aliveRoles[roleDetective] && !containsString(state.NightReadyRoles, "Detective") {
		simRoles = append(simRoles, "Detective")
	}
	if state.Config.HasDoctor && !aliveRoles[roleDoctor] && !containsString(state.NightReadyRoles, "Doctor") {
		simRoles = append(simRoles, "Doctor")
	}
	// Sheriff is never simulated, even if dead

	mafiaAlive := 0
	for _, p := range state.Players {
		if p.Alive && (state.roles[p.ID] == roleMafia || state.roles[p.ID] == roleGodfather) {
			mafiaAlive++
		}
	}
	mafiaPending := state.Config.NumMafia - mafiaAlive - state.MafiaReadyCount
	if mafiaPending < 0 {
		mafiaPending = 0
	}

	room.Mu.RUnlock()

	for _, roleName := range simRoles {
		delay := 2000 + rand.Intn(6000)
		key := fmt.Sprintf("sim_%s", roleName)
		g.scheduleTimer(roomID, key, delay, func() {
			g.applySimulatedReady(roomID, roleName, false)
		})
	}

	for i := 0; i < mafiaPending; i++ {
		delay := 2000 + rand.Intn(6000)
		key := fmt.Sprintf("sim_mafia_%d", i)
		g.scheduleTimer(roomID, key, delay, func() {
			g.applySimulatedReady(roomID, "", true)
		})
	}
}

func (g *Mafia) applySimulatedReady(roomID string, roleName string, isMafia bool) {
	room, err := g.lobby.GetRoom(roomID)
	if err != nil {
		return
	}
	room.Mu.Lock()
	state, ok := room.GameData.(*GameState)
	if !ok || state.Phase != phaseNight {
		room.Mu.Unlock()
		return
	}
	if isMafia {
		state.MafiaReadyCount++
	} else {
		state.NightReadyRoles = addUnique(state.NightReadyRoles, roleName)
	}
	room.Mu.Unlock()
	g.broadcast(room, state)
	g.lobby.PersistRoomAndState(room)
}

func snapshotSessions(room *lobby.GameRoom) map[int64]*session.Session {
	m := make(map[int64]*session.Session, len(room.Participants))
	for id, s := range room.Participants {
		m[id] = s
	}
	return m
}

// ─── Serialization Helpers ──────────────────────────────────────────────────

func rolesMapToJSON(roles map[int64]string) map[string]string {
	m := make(map[string]string, len(roles))
	for k, v := range roles {
		m[strconv.FormatInt(k, 10)] = v
	}
	return m
}

func rolesMapFromJSON(m map[string]string) map[int64]string {
	result := make(map[int64]string, len(m))
	for k, v := range m {
		id, _ := strconv.ParseInt(k, 10, 64)
		result[id] = v
	}
	return result
}

func nightActionsToJSON(actions map[int64]*NightActionData) map[string]*NightActionData {
	m := make(map[string]*NightActionData, len(actions))
	for k, v := range actions {
		m[strconv.FormatInt(k, 10)] = v
	}
	return m
}

func nightActionsFromJSON(m map[string]*NightActionData) map[int64]*NightActionData {
	result := make(map[int64]*NightActionData, len(m))
	for k, v := range m {
		id, _ := strconv.ParseInt(k, 10, 64)
		result[id] = v
	}
	return result
}

func votesMapToJSON(votes map[int64]int64) map[string]int64 {
	m := make(map[string]int64, len(votes))
	for k, v := range votes {
		m[strconv.FormatInt(k, 10)] = v
	}
	return m
}

func votesMapFromJSON(m map[string]int64) map[int64]int64 {
	result := make(map[int64]int64, len(m))
	for k, v := range m {
		id, _ := strconv.ParseInt(k, 10, 64)
		result[id] = v
	}
	return result
}

// ─── dynamicJoinState ────────────────────────────────────────────────────────

func (s *GameState) MaxPlayers() int        { return MaxPlayers }
func (s *GameState) CanJoinDuringPlay() bool { return false }
func (s *GameState) AddParticipant(_ int64) bool { return false }

func (s *GameState) GetPlayerChatRole(playerID int64) string {
	role, ok := s.roles[playerID]
	if !ok {
		return "player"
	}
	if role == roleMafia || role == roleGodfather {
		return "team:mafia"
	}
	for _, p := range s.Players {
		if p.ID == playerID && !p.Alive {
			return "dead"
		}
	}
	for _, p := range s.DeadPlayers {
		if p.ID == playerID {
			return "dead"
		}
	}
	return "player"
}

func (s *GameState) RemoveParticipant(uid int64) {
	if s.Status != "playing" {
		return
	}
	p := findAlivePlayer(s.Players, uid)
	if p == nil {
		return
	}
	role := s.roles[uid]
	deadP := *p
	deadP.Alive = false
	deadP.Role = role
	s.DeadPlayers = append(s.DeadPlayers, deadP)
	s.Players = removePlayerFromAlive(s.Players, uid)

	msg := fmt.Sprintf("💀 %s left the game and was eliminated", p.Username)
	if s.Config.RevealOnDeath {
		msg = fmt.Sprintf("💀 %s left the game and was eliminated (role: %s)", p.Username, roleLabel(role))
	}
	addHistoryWithTarget(s, uid, p.Username, msg)
	s.LastEvent = msg
	s.LastNightEvent = msg

	if won, winner, reason := checkWin(s); won {
		s.Status = "finished"
		s.Phase = phaseGameOver
		s.Winner = winner
		s.WinReason = reason
		s.LastEvent = formatWinMessage(winner, reason)
		s.PhaseEndsAt = 0
		s.nightActions = make(map[int64]*NightActionData)
		s.NightReadyRoles = []string{}
	}

	if s.onDisconnect != nil {
		s.onDisconnect(uid)
	}
}
