package almuamara

import (
	"encoding/json"
	"fmt"
	"math/rand"
	"sort"
	"strconv"
	"sync"
	"time"

	"github.com/cinnabar-games/backend/internal/auth"
	"github.com/cinnabar-games/backend/internal/lobby"
	"github.com/cinnabar-games/backend/internal/protocol"
	"github.com/lonng/nano/component"
	"github.com/lonng/nano/session"
)

const (
	MinPlayers = 6
	MaxPlayers = 12

	startingMs    = 3000
	voteResultMs  = 5000
	cardRevealMs  = 5000
	votingMs      = 120000
	cardActionMs  = 90000
	specialPowerMs = 90000

	phaseStarting            = "starting"
	phaseConsultantSelection = "consultant_selection"
	phaseVoting              = "voting"
	phaseVoteResult          = "vote_result"
	phaseLeaderCard          = "leader_card"
	phaseConsultantCard      = "consultant_card"
	phaseCardReveal          = "card_reveal"
	phaseSpecialPower        = "special_power"
	phaseGameOver            = "game_over"

	roleCitizen   = "citizen"
	roleCriminal  = "criminal"
	roleAssistant = "assistant"

	powerInvestigation    = "investigation"
	powerLeaderSelection  = "leader_selection"
	powerExecution        = "execution"

	winCitizens  = "citizens"
	winCriminals = "criminals"
)

// ─── Types ───────────────────────────────────────────────────────────────────

type Player struct {
	ID       int64  `json:"id"`
	Username string `json:"username"`
}

type HistoryEntry struct {
	Round          int             `json:"round"`
	LeaderID       int64           `json:"leader_id"`
	LeaderName     string          `json:"leader_name"`
	ConsultantID   int64           `json:"consultant_id"`
	ConsultantName string          `json:"consultant_name"`
	CardPlayed     string          `json:"card_played"`
	VoteResult     string          `json:"vote_result"`
	Votes          map[string]bool `json:"votes,omitempty"`
}

type GameState struct {
	Players     []Player `json:"players"`
	DeadPlayers []Player `json:"dead_players"`

	Phase string `json:"phase"`
	Round int    `json:"round"`

	LeaderID     int64 `json:"leader_id"`
	ConsultantID int64 `json:"consultant_id"`

	RecentLeaders     []int64 `json:"recent_leaders"`
	RecentConsultants []int64 `json:"recent_consultants"`

	VotesIn     int             `json:"votes_in"`
	TotalVoters int             `json:"total_voters"`
	VoteResults map[string]bool `json:"vote_results,omitempty"`
	FailedVotes int             `json:"failed_votes"`
	ForceApprove bool           `json:"force_approve"`
	LastVoteApproved bool      `json:"last_vote_approved"`

	RedCardsPlayed   int    `json:"red_cards_played"`
	GreenCardsPlayed int    `json:"green_cards_played"`
	RevealedCard     string `json:"revealed_card"`
	CardCount        int    `json:"card_count"`

	ActiveSpecialPower string `json:"active_special_power"`

	History []HistoryEntry `json:"history"`

	Status    string `json:"status"`
	Winner    string `json:"winner"`
	WinReason string `json:"win_reason"`
	LastEvent string `json:"last_event"`

	PlayerCount int `json:"player_count"`

	// private: serialized via MarshalState / UnmarshalJSON
	roles      map[int64]string
	drawnCards []string
	votes      map[int64]bool
}

func (s *GameState) GetStatus() string { return s.Status }


// ─── Serialization ───────────────────────────────────────────────────────────

func (s *GameState) MarshalState() ([]byte, error) {
	type Alias GameState
	return json.Marshal(&struct {
		Roles      map[string]string `json:"_roles"`
		DrawnCards []string          `json:"_drawn_cards"`
		Votes      map[string]bool   `json:"_votes"`
		*Alias
	}{
		Roles:      rolesMapToJSON(s.roles),
		DrawnCards: s.drawnCards,
		Votes:      votesMapToJSON(s.votes),
		Alias:      (*Alias)(s),
	})
}

func (s *GameState) UnmarshalJSON(data []byte) error {
	type Alias GameState
	aux := &struct {
		Roles      map[string]string `json:"_roles"`
		DrawnCards []string          `json:"_drawn_cards"`
		Votes      map[string]bool   `json:"_votes"`
		*Alias
	}{Alias: (*Alias)(s)}
	if err := json.Unmarshal(data, aux); err != nil {
		return err
	}
	s.roles = rolesMapFromJSON(aux.Roles)
	s.drawnCards = orEmpty(aux.DrawnCards)
	s.votes = votesMapFromJSON(aux.Votes)
	return nil
}

// ─── Private payloads ────────────────────────────────────────────────────────

type CardsPayload struct {
	Cards []string `json:"cards"`
	Phase string   `json:"phase"`
}

type RolePayload struct {
	Role            string   `json:"role"`
	KnownTeammates  []Player `json:"known_teammates"`
}

type InvestigatePayload struct {
	TargetID       int64  `json:"target_id"`
	TargetUsername string `json:"target_username"`
	IsCriminalTeam bool   `json:"is_criminal_team"`
}

// ─── Request types ───────────────────────────────────────────────────────────

type RoomReq struct {
	RoomID string `json:"room_id"`
}

type SelectConsultantReq struct {
	RoomID   string `json:"room_id"`
	TargetID int64  `json:"target_id"`
}

type CastVoteReq struct {
	RoomID  string `json:"room_id"`
	Approve bool   `json:"approve"`
}

type EliminateCardReq struct {
	RoomID    string `json:"room_id"`
	CardIndex int    `json:"card_index"`
}

type UseSpecialPowerReq struct {
	RoomID   string `json:"room_id"`
	TargetID int64  `json:"target_id,omitempty"`
}

// ─── Component ───────────────────────────────────────────────────────────────

type AlMuamara struct {
	component.Base
	lobby  *lobby.Lobby
	mu     sync.Mutex
	timers map[string]*time.Timer
}

func NewAlMuamara(lob *lobby.Lobby) *AlMuamara {
	return &AlMuamara{
		lobby:  lob,
		timers: make(map[string]*time.Timer),
	}
}

func BlankState() *GameState {
	return &GameState{
		Players:           []Player{},
		DeadPlayers:       []Player{},
		History:           []HistoryEntry{},
		RecentLeaders:     []int64{},
		RecentConsultants: []int64{},
		roles:             make(map[int64]string),
		drawnCards:        []string{},
		votes:             make(map[int64]bool),
		Status:            "playing",
	}
}

// ─── InitGame ────────────────────────────────────────────────────────────────

func (g *AlMuamara) InitGame(room *lobby.GameRoom) error {
	room.Mu.Lock()
	n := len(room.Participants)
	if n < MinPlayers {
		room.Mu.Unlock()
		return fmt.Errorf("need at least %d players", MinPlayers)
	}
	if n > MaxPlayers {
		room.Mu.Unlock()
		return fmt.Errorf("maximum %d players allowed", MaxPlayers)
	}

	players := make([]Player, 0, n)
	sessions := make(map[int64]*session.Session, n)
	for pid, sess := range room.Participants {
		players = append(players, Player{ID: pid, Username: sess.String("username")})
		sessions[pid] = sess
	}
	rand.Shuffle(len(players), func(i, j int) { players[i], players[j] = players[j], players[i] })

	roles := distributeRoles(players)
	leaderID := players[rand.Intn(len(players))].ID

	state := &GameState{
		Players:           players,
		DeadPlayers:       []Player{},
		Phase:             phaseStarting,
		Round:             0,
		LeaderID:          leaderID,
		ConsultantID:      0,
		RecentLeaders:     []int64{},
		RecentConsultants: []int64{},
		VotesIn:           0,
		TotalVoters:       n,
		FailedVotes:       0,
		ForceApprove:      false,
		RedCardsPlayed:    0,
		GreenCardsPlayed:  0,
		RevealedCard:      "",
		CardCount:         0,
		History:           []HistoryEntry{},
		Status:            "playing",
		LastEvent:         "Roles have been assigned. The game is starting!",
		PlayerCount:       n,
		roles:             roles,
		drawnCards:        []string{},
		votes:             make(map[int64]bool),
	}

	room.GameData = state
	roomID := room.ID
	room.Mu.Unlock()

	// push private role info to each player
	for _, p := range state.Players {
		if sess, ok := sessions[p.ID]; ok {
			payload := buildRolePayload(p.ID, state)
			sess.Push("onMuamaraRole", payload)
		}
	}

	g.broadcast(room, state)
	g.lobby.PersistRoomAndState(room)
	g.scheduleTimer(roomID, "starting", startingMs, func() {
		g.advanceFromStarting(roomID)
	})
	return nil
}

// ─── RestoreGame ─────────────────────────────────────────────────────────────

func (g *AlMuamara) RestoreGame(room *lobby.GameRoom) {
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
	room.Mu.RUnlock()

	switch phase {
	case phaseStarting:
		g.scheduleTimer(roomID, "starting", startingMs, func() {
			g.advanceFromStarting(roomID)
		})
	case phaseVoting:
		g.scheduleTimer(roomID, "voting", votingMs, func() {
			g.resolveVotingTimeout(roomID)
		})
	case phaseVoteResult:
		g.scheduleTimer(roomID, "vote_result", voteResultMs, func() {
			g.advanceFromVoteResult(roomID)
		})
	case phaseLeaderCard:
		g.scheduleTimer(roomID, "leader_card", cardActionMs, func() {
			g.autoEliminateLeaderCard(roomID)
		})
	case phaseConsultantCard:
		g.scheduleTimer(roomID, "consultant_card", cardActionMs, func() {
			g.autoEliminateConsultantCard(roomID)
		})
	case phaseCardReveal:
		g.scheduleTimer(roomID, "card_reveal", cardRevealMs, func() {
			g.advanceFromCardReveal(roomID)
		})
	case phaseSpecialPower:
		g.scheduleTimer(roomID, "special_power", specialPowerMs, func() {
			g.skipSpecialPower(roomID)
		})
	}
}

// ─── Handlers ────────────────────────────────────────────────────────────────

func (g *AlMuamara) GetState(s *session.Session, req *RoomReq) error {
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

	// push private info to reconnecting player
	if uid != 0 {
		if state.roles != nil && len(state.roles) > 0 {
			payload := buildRolePayload(uid, state)
			s.Push("onMuamaraRole", payload)
		}
		g.pushCardsToPlayer(s, uid, state)
	}
	return nil
}

func (g *AlMuamara) GetPrivateInfo(s *session.Session, req *RoomReq) error {
	uid, _, err := auth.BindPlayer(s)
	if err != nil {
		return s.Response(&protocol.Response{Code: 401, Message: err.Error()})
	}
	room, err := g.lobby.GetRoom(req.RoomID)
	if err != nil {
		return s.Response(&protocol.Response{Code: 404, Message: "room not found"})
	}
	room.Mu.RLock()
	state, ok := room.GameData.(*GameState)
	room.Mu.RUnlock()
	if !ok {
		return s.Response(&protocol.Response{Code: 404, Message: "no state"})
	}
	payload := buildRolePayload(uid, state)
	return s.Response(payload)
}

func (g *AlMuamara) SelectConsultant(s *session.Session, req *SelectConsultantReq) error {
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
	if state.Phase != phaseConsultantSelection {
		room.Mu.Unlock()
		return s.Response(&protocol.Response{Code: 400, Message: "not consultant selection phase"})
	}
	if state.LeaderID != uid {
		room.Mu.Unlock()
		return s.Response(&protocol.Response{Code: 403, Message: "only the leader can select a consultant"})
	}
	if req.TargetID == uid {
		room.Mu.Unlock()
		return s.Response(&protocol.Response{Code: 400, Message: "cannot select yourself as consultant"})
	}
	if findPlayer(state.Players, req.TargetID) == nil {
		room.Mu.Unlock()
		return s.Response(&protocol.Response{Code: 400, Message: "target player not in game"})
	}

	// check if target is eligible (not prev leader or prev consultant)
	if !isConsultantEligible(req.TargetID, state) {
		// if all eligible players are exhausted, relax constraint
		eligible := consultantEligiblePlayers(state)
		if len(eligible) > 0 {
			room.Mu.Unlock()
			return s.Response(&protocol.Response{Code: 400, Message: "this player is ineligible as consultant this round"})
		}
	}

	state.ConsultantID = req.TargetID
	consultant := findPlayer(state.Players, req.TargetID)
	leader := findPlayer(state.Players, uid)
	state.LastEvent = fmt.Sprintf("%s selected %s as consultant", leader.Username, consultant.Username)

	sessions := snapshotSessions(room)
	roomID := room.ID

	if state.ForceApprove {
		// skip voting, go directly to card draw
		state.VoteResults = nil
		state.VotesIn = 0
		state.TotalVoters = len(state.Players)
		state.Round++
		g.enterLeaderCardPhase(state, sessions)
		room.Mu.Unlock()
		g.broadcast(room, state)
		g.lobby.PersistRoomAndState(room)
		g.scheduleTimer(roomID, "leader_card", cardActionMs, func() {
			g.autoEliminateLeaderCard(roomID)
		})
	} else {
		state.Phase = phaseVoting
		state.VotesIn = 0
		state.TotalVoters = countAlivePlayers(state)
		state.votes = make(map[int64]bool)
		state.VoteResults = nil
		state.Round++
		leaderName := leader.Username
		consultantName := consultant.Username
		state.LastEvent = fmt.Sprintf("Vote on %s + %s team — approve or reject!", leaderName, consultantName)
		room.Mu.Unlock()
		g.broadcast(room, state)
		g.lobby.PersistRoomAndState(room)
		g.scheduleTimer(roomID, "voting", votingMs, func() {
			g.resolveVotingTimeout(roomID)
		})
	}

	return s.Response(&protocol.Response{Code: 0})
}

func (g *AlMuamara) CastVote(s *session.Session, req *CastVoteReq) error {
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
	if state.Phase != phaseVoting {
		room.Mu.Unlock()
		return s.Response(&protocol.Response{Code: 400, Message: "not voting phase"})
	}
	if findPlayer(state.Players, uid) == nil {
		room.Mu.Unlock()
		return s.Response(&protocol.Response{Code: 403, Message: "you are not an active player"})
	}
	if _, already := state.votes[uid]; already {
		room.Mu.Unlock()
		return s.Response(&protocol.Response{Code: 400, Message: "already voted"})
	}

	state.votes[uid] = req.Approve
	state.VotesIn++

	allIn := state.VotesIn >= countAlivePlayers(state)
	room.Mu.Unlock()
	g.broadcast(room, state)

	if allIn {
		g.cancelTimer(room.ID)
		g.resolveVotes(room)
	}

	return s.Response(&protocol.Response{Code: 0})
}

func (g *AlMuamara) EliminateCard(s *session.Session, req *EliminateCardReq) error {
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

	sessions := snapshotSessions(room)
	roomID := room.ID

	if state.Phase == phaseLeaderCard {
		if uid != state.LeaderID {
			room.Mu.Unlock()
			return s.Response(&protocol.Response{Code: 403, Message: "only the leader can eliminate a card now"})
		}
		if req.CardIndex < 0 || req.CardIndex >= len(state.drawnCards) {
			room.Mu.Unlock()
			return s.Response(&protocol.Response{Code: 400, Message: "invalid card index"})
		}
		// remove the selected card
		state.drawnCards = append(state.drawnCards[:req.CardIndex], state.drawnCards[req.CardIndex+1:]...)
		state.Phase = phaseConsultantCard
		state.CardCount = len(state.drawnCards)
		leader := findPlayer(state.Players, state.LeaderID)
		consultant := findPlayer(state.Players, state.ConsultantID)
		state.LastEvent = fmt.Sprintf("%s eliminated a card — %s is choosing now", leader.Username, consultant.Username)

		// push remaining 2 cards to consultant
		if csess, ok := sessions[state.ConsultantID]; ok {
			csess.Push("onMuamaraCards", &CardsPayload{Cards: state.drawnCards, Phase: phaseConsultantCard})
		}
		room.Mu.Unlock()
		g.cancelTimer(roomID)
		g.broadcast(room, state)
		g.lobby.PersistRoomAndState(room)
		g.scheduleTimer(roomID, "consultant_card", cardActionMs, func() {
			g.autoEliminateConsultantCard(roomID)
		})

	} else if state.Phase == phaseConsultantCard {
		if uid != state.ConsultantID {
			room.Mu.Unlock()
			return s.Response(&protocol.Response{Code: 403, Message: "only the consultant can eliminate a card now"})
		}
		if req.CardIndex < 0 || req.CardIndex >= len(state.drawnCards) {
			room.Mu.Unlock()
			return s.Response(&protocol.Response{Code: 400, Message: "invalid card index"})
		}
		// remove selected card — remaining card is the played one
		state.drawnCards = append(state.drawnCards[:req.CardIndex], state.drawnCards[req.CardIndex+1:]...)
		playedCard := state.drawnCards[0]
		state.drawnCards = []string{}
		state.RevealedCard = playedCard
		state.CardCount = 0

		consultant := findPlayer(state.Players, state.ConsultantID)
		leader := findPlayer(state.Players, state.LeaderID)

		// update scores & history
		var specialPower string
		if playedCard == "red" {
			state.RedCardsPlayed++
			specialPower = getSpecialPower(state.RedCardsPlayed, state.PlayerCount)
			state.LastEvent = fmt.Sprintf("🔴 Red card played! Criminals score. (%d/6)", state.RedCardsPlayed)
		} else {
			state.GreenCardsPlayed++
			state.LastEvent = fmt.Sprintf("🟢 Green card played! Citizens score. (%d/5)", state.GreenCardsPlayed)
		}

		// record history
		voteMap := make(map[string]bool, len(state.votes))
		for pid, v := range state.votes {
			voteMap[strconv.FormatInt(pid, 10)] = v
		}
		state.History = append(state.History, HistoryEntry{
			Round:          state.Round,
			LeaderID:       state.LeaderID,
			LeaderName:     leader.Username,
			ConsultantID:   state.ConsultantID,
			ConsultantName: consultant.Username,
			CardPlayed:     playedCard,
			VoteResult:     "approved",
			Votes:          voteMap,
		})

		// reset force approve after one successful round
		state.ForceApprove = false

		// check win conditions before special power
		if won, winner, reason := checkWin(state); won {
			state.Status = "finished"
			state.Phase = phaseGameOver
			state.Winner = winner
			state.WinReason = reason
			if winner == winCitizens {
				state.LastEvent = "🛡️ Citizens win! " + reason
			} else {
				state.LastEvent = "💀 Criminals win! " + reason
			}
			room.Mu.Unlock()
			g.cancelTimer(roomID)
			g.broadcast(room, state)
			room.Mu.Lock()
			room.Status = "finished"
			room.Mu.Unlock()
			g.lobby.PersistRoomAndState(room)
			return s.Response(&protocol.Response{Code: 0})
		}

		// trigger special power or go to reveal
		if specialPower != "" {
			state.ActiveSpecialPower = specialPower
			state.Phase = phaseCardReveal
			state.CardCount = 0
		} else {
			state.ActiveSpecialPower = ""
			state.Phase = phaseCardReveal
		}

		room.Mu.Unlock()
		g.cancelTimer(roomID)
		g.broadcast(room, state)
		g.lobby.PersistRoomAndState(room)
		g.scheduleTimer(roomID, "card_reveal", cardRevealMs, func() {
			g.advanceFromCardReveal(roomID)
		})

	} else {
		room.Mu.Unlock()
		return s.Response(&protocol.Response{Code: 400, Message: "not a card elimination phase"})
	}

	return s.Response(&protocol.Response{Code: 0})
}

func (g *AlMuamara) UseSpecialPower(s *session.Session, req *UseSpecialPowerReq) error {
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
	if state.Phase != phaseSpecialPower {
		room.Mu.Unlock()
		return s.Response(&protocol.Response{Code: 400, Message: "no active special power"})
	}
	if uid != state.LeaderID {
		room.Mu.Unlock()
		return s.Response(&protocol.Response{Code: 403, Message: "only the leader can use the special power"})
	}

	sessions := snapshotSessions(room)
	roomID := room.ID
	power := state.ActiveSpecialPower
	leaderSess := sessions[uid]

	switch power {
	case powerInvestigation:
		if req.TargetID == 0 {
			room.Mu.Unlock()
			return s.Response(&protocol.Response{Code: 400, Message: "target required for investigation"})
		}
		target := findPlayer(state.Players, req.TargetID)
		if target == nil {
			room.Mu.Unlock()
			return s.Response(&protocol.Response{Code: 400, Message: "target not in game"})
		}
		targetRole := state.roles[req.TargetID]
		isCriminalTeam := targetRole == roleCriminal || targetRole == roleAssistant
		state.LastEvent = fmt.Sprintf("🔍 %s used Investigation on %s", findPlayer(state.Players, uid).Username, target.Username)
		room.Mu.Unlock()
		g.cancelTimer(roomID)
		// push result privately to leader only
		if leaderSess != nil {
			leaderSess.Push("onMuamaraInvestigate", &InvestigatePayload{
				TargetID:       req.TargetID,
				TargetUsername: target.Username,
				IsCriminalTeam: isCriminalTeam,
			})
		}
		g.broadcast(room, state)
		g.lobby.PersistRoomAndState(room)
		// advance to next round
		g.startNextRound(roomID)

	case powerLeaderSelection:
		// current leader stays as next leader
		state.LastEvent = fmt.Sprintf("👑 %s used Leader Selection — they stay as leader!", findPlayer(state.Players, uid).Username)
		// mark: on next round, force same leader (bypass rotation)
		state.RecentLeaders = prependInt64(state.RecentLeaders, state.LeaderID, 2)
		state.RecentConsultants = prependInt64(state.RecentConsultants, state.ConsultantID, 2)
		nextLeaderID := state.LeaderID // keep same leader
		state.ActiveSpecialPower = ""
		room.Mu.Unlock()
		g.cancelTimer(roomID)
		g.broadcast(room, state)
		g.lobby.PersistRoomAndState(room)
		g.beginConsultantSelection(roomID, nextLeaderID)

	case powerExecution:
		if req.TargetID == 0 {
			room.Mu.Unlock()
			return s.Response(&protocol.Response{Code: 400, Message: "target required for execution"})
		}
		target := findPlayer(state.Players, req.TargetID)
		if target == nil {
			room.Mu.Unlock()
			return s.Response(&protocol.Response{Code: 400, Message: "target not in game"})
		}
		targetRole := state.roles[req.TargetID]
		leaderName := findPlayer(state.Players, uid).Username

		// kill the target
		state.Players = removePlayerFromSlice(state.Players, req.TargetID)
		state.DeadPlayers = append(state.DeadPlayers, *target)
		// remove dead player from recent history so they don't block eligibility
		state.RecentLeaders = filterInt64(state.RecentLeaders, req.TargetID)
		state.RecentConsultants = filterInt64(state.RecentConsultants, req.TargetID)

		if targetRole == roleCriminal {
			// citizens win instantly
			state.Status = "finished"
			state.Phase = phaseGameOver
			state.Winner = winCitizens
			state.WinReason = fmt.Sprintf("%s executed the Criminal %s!", leaderName, target.Username)
			state.LastEvent = fmt.Sprintf("💀 %s was executed — and they were the Criminal! Citizens win!", target.Username)
			room.Mu.Unlock()
			g.cancelTimer(roomID)
			g.broadcast(room, state)
			room.Mu.Lock()
			room.Status = "finished"
			room.Mu.Unlock()
			g.lobby.PersistRoomAndState(room)
		} else {
			state.LastEvent = fmt.Sprintf("💀 %s executed %s — they were not the Criminal!", leaderName, target.Username)
			state.ActiveSpecialPower = ""
			room.Mu.Unlock()
			g.cancelTimer(roomID)
			g.broadcast(room, state)
			g.lobby.PersistRoomAndState(room)
			g.startNextRound(roomID)
		}

	default:
		room.Mu.Unlock()
		return s.Response(&protocol.Response{Code: 400, Message: "unknown special power"})
	}

	return s.Response(&protocol.Response{Code: 0})
}

func (g *AlMuamara) SkipSpecialPower(s *session.Session, req *RoomReq) error {
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
	if state.Phase != phaseSpecialPower {
		room.Mu.Unlock()
		return s.Response(&protocol.Response{Code: 400, Message: "no active special power"})
	}
	if uid != state.LeaderID {
		room.Mu.Unlock()
		return s.Response(&protocol.Response{Code: 403, Message: "only the leader can skip the power"})
	}
	state.LastEvent = fmt.Sprintf("%s chose not to use the special power", findPlayer(state.Players, uid).Username)
	state.ActiveSpecialPower = ""
	room.Mu.Unlock()
	g.cancelTimer(room.ID)
	g.broadcast(room, state)
	g.lobby.PersistRoomAndState(room)
	g.startNextRound(room.ID)
	return s.Response(&protocol.Response{Code: 0})
}

func (g *AlMuamara) ResetLeaderSelection(s *session.Session, req *RoomReq) error {
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
		return s.Response(&protocol.Response{Code: 403, Message: "only the host can reset leader selection"})
	}
	state, ok := room.GameData.(*GameState)
	if !ok || state.Status != "playing" {
		room.Mu.Unlock()
		return s.Response(&protocol.Response{Code: 400, Message: "game not active"})
	}
allowedPhases := map[string]bool{
		phaseConsultantSelection: true,
		phaseVoting:              true,
		phaseStarting:            true,
	}
	if !allowedPhases[state.Phase] {
		room.Mu.Unlock()
		return s.Response(&protocol.Response{Code: 400, Message: "can only reset during leader/consultant selection or voting"})
	}

	wasVoting := state.Phase == phaseVoting

	// pick a new leader (does NOT count as a failed vote)
	newLeaderID := selectNextLeader(state)
	state.LeaderID = newLeaderID
	state.ConsultantID = 0
	state.Phase = phaseConsultantSelection
	state.votes = make(map[int64]bool)
	state.VoteResults = nil
	state.VotesIn = 0
	// roll back round increment if we were in voting
	if wasVoting && state.Round > 0 {
		state.Round--
	}
	leader := findPlayer(state.Players, newLeaderID)
	if leader != nil {
		state.LastEvent = fmt.Sprintf("Leader reset by host — %s is now the leader", leader.Username)
	}
	roomID := room.ID
	room.Mu.Unlock()

	g.cancelTimer(roomID)
	g.broadcast(room, state)
	g.lobby.PersistRoomAndState(room)
	return s.Response(&protocol.Response{Code: 0})
}

// ─── Phase machine ───────────────────────────────────────────────────────────

func (g *AlMuamara) advanceFromStarting(roomID string) {
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
	state.Phase = phaseConsultantSelection
	leader := findPlayer(state.Players, state.LeaderID)
	if leader != nil {
		state.LastEvent = fmt.Sprintf("%s is the leader — choose your consultant!", leader.Username)
	}
	room.Mu.Unlock()
	g.broadcast(room, state)
	g.lobby.PersistRoomAndState(room)
}

func (g *AlMuamara) resolveVotingTimeout(roomID string) {
	room, err := g.lobby.GetRoom(roomID)
	if err != nil {
		return
	}
	room.Mu.Lock()
	state, ok := room.GameData.(*GameState)
	if !ok || state.Phase != phaseVoting {
		room.Mu.Unlock()
		return
	}
	// Count votes already cast; non-voters count as abstain (reject)
	state.VotesIn = countAlivePlayers(state) // treat as all voted
	room.Mu.Unlock()
	g.resolveVotes(room)
}

func (g *AlMuamara) resolveVotes(room *lobby.GameRoom) {
	room.Mu.Lock()
	state, ok := room.GameData.(*GameState)
	if !ok {
		room.Mu.Unlock()
		return
	}

	approvals := 0
	for _, v := range state.votes {
		if v {
			approvals++
		}
	}
	total := countAlivePlayers(state)
	approved := approvals*2 > total // strict majority

	// build vote results for display
	voteResults := make(map[string]bool)
	for pid, v := range state.votes {
		voteResults[strconv.FormatInt(pid, 10)] = v
	}
	state.VoteResults = voteResults

	roomID := room.ID
	sessions := snapshotSessions(room)

	if approved {
		state.FailedVotes = 0
		state.Phase = phaseVoteResult
		state.LastVoteApproved = true
		state.LastEvent = fmt.Sprintf("✅ Vote approved! (%d/%d) — Cards will be played.", approvals, total)
		room.Mu.Unlock()
		g.broadcast(room, state)
		g.lobby.PersistRoomAndState(room)
		g.scheduleTimer(roomID, "vote_result", voteResultMs, func() {
			g.advanceFromVoteResult(roomID)
		})
	} else {
		state.FailedVotes++
		state.Phase = phaseVoteResult
		state.LastVoteApproved = false

if state.FailedVotes >= 2 {
			// auto-play a red card (no special power, no deck manipulation)
			state.RedCardsPlayed++
			state.RevealedCard = "red"
			state.CardCount = 0

			leader := findPlayer(state.Players, state.LeaderID)
			consultant := findPlayer(state.Players, state.ConsultantID)
			voteMap := make(map[string]bool, len(state.votes))
			for pid, v := range state.votes {
				voteMap[strconv.FormatInt(pid, 10)] = v
			}
			state.History = append(state.History, HistoryEntry{
				Round:          state.Round,
				LeaderID:       state.LeaderID,
				LeaderName:     orUnknown(leader),
				ConsultantID:   state.ConsultantID,
				ConsultantName: orUnknown(consultant),
				CardPlayed:     "auto_red",
				VoteResult:     "auto_red",
				Votes:          voteMap,
			})

			state.LastEvent = fmt.Sprintf("❌ 2 consecutive rejections! A red card is auto-played. (%d/6)", state.RedCardsPlayed)
			state.FailedVotes = 0
			state.ForceApprove = true

			// check win immediately
			if won, winner, reason := checkWin(state); won {
				state.Status = "finished"
				state.Phase = phaseGameOver
				state.Winner = winner
				state.WinReason = reason
				if winner == winCitizens {
					state.LastEvent = "🛡️ Citizens win! " + reason
				} else {
					state.LastEvent = "💀 Criminals win! " + reason
				}
				room.Mu.Unlock()
				g.broadcast(room, state)
				room.Mu.Lock()
				room.Status = "finished"
				room.Mu.Unlock()
				g.lobby.PersistRoomAndState(room)
				return
			}
		} else {
			state.LastEvent = fmt.Sprintf("❌ Vote rejected (%d/%d). Failed votes: %d/2.", approvals, total, state.FailedVotes)
		}

		room.Mu.Unlock()
		g.broadcast(room, state)
		g.lobby.PersistRoomAndState(room)
		g.scheduleTimer(roomID, "vote_result", voteResultMs, func() {
			g.advanceFromVoteResult(roomID)
		})
	}
	_ = sessions
}

func (g *AlMuamara) advanceFromVoteResult(roomID string) {
	room, err := g.lobby.GetRoom(roomID)
	if err != nil {
		return
	}
	room.Mu.Lock()
	state, ok := room.GameData.(*GameState)
	if !ok || state.Status != "playing" || state.Phase != phaseVoteResult {
		room.Mu.Unlock()
		return
	}

	if state.LastVoteApproved {
		sessions := snapshotSessions(room)
		g.enterLeaderCardPhase(state, sessions)
		roomID2 := room.ID
		room.Mu.Unlock()
		g.broadcast(room, state)
		g.lobby.PersistRoomAndState(room)
		g.scheduleTimer(roomID2, "leader_card", cardActionMs, func() {
			g.autoEliminateLeaderCard(roomID2)
		})
	} else {
		// rejected: advance leader, back to consultant selection
		g.updateRotationHistory(state)
		nextLeaderID := selectNextLeader(state)
		state.LeaderID = nextLeaderID
		state.ConsultantID = 0
		state.Phase = phaseConsultantSelection
		state.VoteResults = nil
		state.votes = make(map[int64]bool)
		state.VotesIn = 0
		state.RevealedCard = ""
		leader := findPlayer(state.Players, nextLeaderID)
		if leader != nil {
			state.LastEvent = fmt.Sprintf("%s is the new leader — choose your consultant!", leader.Username)
		}
		room.Mu.Unlock()
		g.broadcast(room, state)
		g.lobby.PersistRoomAndState(room)
	}
}

func (g *AlMuamara) enterLeaderCardPhase(state *GameState, sessions map[int64]*session.Session) {
	// build a fresh shuffled deck each round
	deck := buildDeck(state.PlayerCount)
	rand.Shuffle(len(deck), func(i, j int) { deck[i], deck[j] = deck[j], deck[i] })
	drawn := []string{deck[0], deck[1], deck[2]}
	state.drawnCards = drawn
	state.Phase = phaseLeaderCard
	state.CardCount = len(drawn)
	leader := findPlayer(state.Players, state.LeaderID)
	if leader != nil {
		state.LastEvent = fmt.Sprintf("%s is drawing cards — only they can see them!", leader.Username)
	}
	// push cards privately to leader
	if lsess, ok := sessions[state.LeaderID]; ok {
		lsess.Push("onMuamaraCards", &CardsPayload{Cards: drawn, Phase: phaseLeaderCard})
	}
}

func (g *AlMuamara) autoEliminateLeaderCard(roomID string) {
	room, err := g.lobby.GetRoom(roomID)
	if err != nil {
		return
	}
	room.Mu.Lock()
	state, ok := room.GameData.(*GameState)
	if !ok || state.Phase != phaseLeaderCard {
		room.Mu.Unlock()
		return
	}
	if len(state.drawnCards) == 0 {
		room.Mu.Unlock()
		return
	}
	// auto-eliminate first card
	state.drawnCards = state.drawnCards[1:]
	state.Phase = phaseConsultantCard
	state.CardCount = len(state.drawnCards)
	sessions := snapshotSessions(room)
	consultant := findPlayer(state.Players, state.ConsultantID)
	if consultant != nil {
		state.LastEvent = fmt.Sprintf("Time's up! Card auto-eliminated. %s is choosing now.", consultant.Username)
	}
	if csess, ok := sessions[state.ConsultantID]; ok {
		csess.Push("onMuamaraCards", &CardsPayload{Cards: state.drawnCards, Phase: phaseConsultantCard})
	}
	room.Mu.Unlock()
	g.broadcast(room, state)
	g.lobby.PersistRoomAndState(room)
	g.scheduleTimer(roomID, "consultant_card", cardActionMs, func() {
		g.autoEliminateConsultantCard(roomID)
	})
}

func (g *AlMuamara) autoEliminateConsultantCard(roomID string) {
	room, err := g.lobby.GetRoom(roomID)
	if err != nil {
		return
	}
	room.Mu.Lock()
	state, ok := room.GameData.(*GameState)
	if !ok || state.Phase != phaseConsultantCard {
		room.Mu.Unlock()
		return
	}
	if len(state.drawnCards) == 0 {
		room.Mu.Unlock()
		return
	}
	// auto-eliminate first card, remaining is played
	state.drawnCards = state.drawnCards[1:]
	if len(state.drawnCards) == 0 {
		room.Mu.Unlock()
		return
	}
	playedCard := state.drawnCards[0]
	state.drawnCards = []string{}
	state.RevealedCard = playedCard
	state.CardCount = 0

	leader := findPlayer(state.Players, state.LeaderID)
	consultant := findPlayer(state.Players, state.ConsultantID)

	var specialPower string
	if playedCard == "red" {
		state.RedCardsPlayed++
		specialPower = getSpecialPower(state.RedCardsPlayed, state.PlayerCount)
		state.LastEvent = fmt.Sprintf("🔴 Red card played! (auto-timeout) (%d/6)", state.RedCardsPlayed)
	} else {
		state.GreenCardsPlayed++
		state.LastEvent = fmt.Sprintf("🟢 Green card played! (auto-timeout) (%d/5)", state.GreenCardsPlayed)
	}

	voteMap := make(map[string]bool, len(state.votes))
	for pid, v := range state.votes {
		voteMap[strconv.FormatInt(pid, 10)] = v
	}
	state.History = append(state.History, HistoryEntry{
		Round:          state.Round,
		LeaderID:       state.LeaderID,
		LeaderName:     orUnknown(leader),
		ConsultantID:   state.ConsultantID,
		ConsultantName: orUnknown(consultant),
		CardPlayed:     playedCard,
		VoteResult:     "approved",
		Votes:          voteMap,
	})

	state.ForceApprove = false

	if won, winner, reason := checkWin(state); won {
		state.Status = "finished"
		state.Phase = phaseGameOver
		state.Winner = winner
		state.WinReason = reason
		room.Mu.Unlock()
		g.broadcast(room, state)
		room.Mu.Lock()
		room.Status = "finished"
		room.Mu.Unlock()
		g.lobby.PersistRoomAndState(room)
		return
	}

	if specialPower != "" {
		state.ActiveSpecialPower = specialPower
	} else {
		state.ActiveSpecialPower = ""
	}
	state.Phase = phaseCardReveal
	room.Mu.Unlock()
	g.broadcast(room, state)
	g.lobby.PersistRoomAndState(room)
	g.scheduleTimer(roomID, "card_reveal", cardRevealMs, func() {
		g.advanceFromCardReveal(roomID)
	})
}

func (g *AlMuamara) advanceFromCardReveal(roomID string) {
	room, err := g.lobby.GetRoom(roomID)
	if err != nil {
		return
	}
	room.Mu.Lock()
	state, ok := room.GameData.(*GameState)
	if !ok || state.Status != "playing" || state.Phase != phaseCardReveal {
		room.Mu.Unlock()
		return
	}

	if state.ActiveSpecialPower != "" {
		state.Phase = phaseSpecialPower
		power := state.ActiveSpecialPower
		leaderName := ""
		if l := findPlayer(state.Players, state.LeaderID); l != nil {
			leaderName = l.Username
		}
		switch power {
		case powerInvestigation:
			state.LastEvent = fmt.Sprintf("🔍 Special Power: Investigation! %s can reveal a player's role.", leaderName)
		case powerLeaderSelection:
			state.LastEvent = fmt.Sprintf("👑 Special Power: Leader Selection! %s can remain as leader.", leaderName)
		case powerExecution:
			state.LastEvent = fmt.Sprintf("💀 Special Power: Execution! %s can eliminate a player permanently.", leaderName)
		}
		room.Mu.Unlock()
		g.broadcast(room, state)
		g.lobby.PersistRoomAndState(room)
		g.scheduleTimer(roomID, "special_power", specialPowerMs, func() {
			g.skipSpecialPower(roomID)
		})
	} else {
		g.updateRotationHistory(state)
		room.Mu.Unlock()
		g.startNextRound(roomID)
	}
}

func (g *AlMuamara) skipSpecialPower(roomID string) {
	room, err := g.lobby.GetRoom(roomID)
	if err != nil {
		return
	}
	room.Mu.Lock()
	state, ok := room.GameData.(*GameState)
	if !ok || state.Phase != phaseSpecialPower {
		room.Mu.Unlock()
		return
	}
	state.ActiveSpecialPower = ""
	state.LastEvent = "Special power expired — moving to next round."
	room.Mu.Unlock()
	g.startNextRound(roomID)
}

func (g *AlMuamara) startNextRound(roomID string) {
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
	g.updateRotationHistory(state)
	nextLeaderID := selectNextLeader(state)
	g.beginConsultantSelectionState(state, nextLeaderID)
	room.Mu.Unlock()
	g.broadcast(room, state)
	g.lobby.PersistRoomAndState(room)
}

func (g *AlMuamara) beginConsultantSelectionState(state *GameState, leaderID int64) {
	state.LeaderID = leaderID
	state.ConsultantID = 0
	state.Phase = phaseConsultantSelection
	state.votes = make(map[int64]bool)
	state.VoteResults = nil
	state.VotesIn = 0
	state.RevealedCard = ""
	state.drawnCards = []string{}
	state.CardCount = 0
	state.ActiveSpecialPower = ""
	leader := findPlayer(state.Players, leaderID)
	if leader != nil {
		state.LastEvent = fmt.Sprintf("%s is the leader — choose your consultant!", leader.Username)
	}
}

func (g *AlMuamara) beginConsultantSelection(roomID string, leaderID int64) {
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
	g.beginConsultantSelectionState(state, leaderID)
	room.Mu.Unlock()
	g.broadcast(room, state)
	g.lobby.PersistRoomAndState(room)
}

// ─── Game logic helpers ───────────────────────────────────────────────────────

func distributeRoles(players []Player) map[int64]string {
	n := len(players)
	roles := make(map[int64]string, n)

	var numCriminals, numAssistants int
	switch {
	case n <= 7:
		numCriminals, numAssistants = 1, 1
	case n == 8:
		numCriminals, numAssistants = 1, 2
	case n == 9:
		numCriminals, numAssistants = 1, 2
	case n == 10:
		numCriminals, numAssistants = 1, 3
	case n == 11:
		numCriminals, numAssistants = 1, 3
	default: // 12
		numCriminals, numAssistants = 1, 4
	}

	indices := rand.Perm(n)
	ptr := 0
	for i := 0; i < numCriminals; i++ {
		roles[players[indices[ptr]].ID] = roleCriminal
		ptr++
	}
	for i := 0; i < numAssistants; i++ {
		roles[players[indices[ptr]].ID] = roleAssistant
		ptr++
	}
	for ; ptr < n; ptr++ {
		roles[players[indices[ptr]].ID] = roleCitizen
	}
	return roles
}

func buildRolePayload(playerID int64, state *GameState) *RolePayload {
	role := state.roles[playerID]
	teammates := []Player{}

	n := state.PlayerCount

	switch role {
	case roleCriminal:
		// criminal knows all assistants
		for _, p := range state.Players {
			r := state.roles[p.ID]
			if p.ID != playerID && r == roleAssistant {
				teammates = append(teammates, p)
			}
		}
		for _, p := range state.DeadPlayers {
			r := state.roles[p.ID]
			if p.ID != playerID && r == roleAssistant {
				teammates = append(teammates, p)
			}
		}
	case roleAssistant:
		// find the criminal (always known to assistant)
		for _, p := range append(state.Players, state.DeadPlayers...) {
			if state.roles[p.ID] == roleCriminal {
				teammates = append(teammates, p)
				break
			}
		}
		if n <= 10 {
			// all assistants know each other
			for _, p := range append(state.Players, state.DeadPlayers...) {
				if p.ID != playerID && state.roles[p.ID] == roleAssistant {
					teammates = append(teammates, p)
				}
			}
		} else if n == 12 {
			// each assistant knows criminal + one other assistant (paired by sorted ID)
			allAssistants := []Player{}
			for _, p := range append(state.Players, state.DeadPlayers...) {
				if state.roles[p.ID] == roleAssistant {
					allAssistants = append(allAssistants, p)
				}
			}
			sort.Slice(allAssistants, func(i, j int) bool { return allAssistants[i].ID < allAssistants[j].ID })
			for i, p := range allAssistants {
				if p.ID == playerID {
					pairIdx := i ^ 1
					if pairIdx < len(allAssistants) {
						partner := allAssistants[pairIdx]
						if partner.ID != playerID {
							teammates = append(teammates, partner)
						}
					}
					break
				}
			}
		}
		// n==11: assistants only know the criminal (already added above)
	case roleCitizen:
		// citizens know nothing
	}

	return &RolePayload{Role: role, KnownTeammates: teammates}
}

func buildDeck(n int) []string {
	var red, green int
	switch {
	case n <= 8:
		red, green = 11, 6
	case n <= 10:
		red, green = 13, 6
	default:
		red, green = 15, 6
	}
	deck := make([]string, 0, red+green)
	for i := 0; i < red; i++ {
		deck = append(deck, "red")
	}
	for i := 0; i < green; i++ {
		deck = append(deck, "green")
	}
	return deck
}

func getSpecialPower(redCount, playerCount int) string {
	switch redCount {
	case 2:
		return powerInvestigation
	case 4:
		return powerLeaderSelection
	case 5:
		if playerCount >= 9 {
			return powerExecution
		}
	}
	return ""
}

func checkWin(state *GameState) (bool, string, string) {
	if state.GreenCardsPlayed >= 5 {
		return true, winCitizens, "Citizens played 5 green cards!"
	}
	if state.RedCardsPlayed >= 6 {
		return true, winCriminals, "Criminals played 6 red cards!"
	}
	return false, "", ""
}

func selectNextLeader(state *GameState) int64 {
	if len(state.Players) == 0 {
		return 0
	}

	// find current leader's position for sequential rotation
	currentIdx := -1
	for i, p := range state.Players {
		if p.ID == state.LeaderID {
			currentIdx = i
			break
		}
	}
	if currentIdx == -1 {
		currentIdx = 0
	}

	// build ineligible set based on player count
	ineligible := make(map[int64]bool)
	n := state.PlayerCount

	switch {
	case n <= 8:
		if len(state.RecentConsultants) > 0 {
			ineligible[state.RecentConsultants[0]] = true
		}
	case n <= 11:
		if len(state.RecentLeaders) > 0 {
			ineligible[state.RecentLeaders[0]] = true
		}
		if len(state.RecentConsultants) > 0 {
			ineligible[state.RecentConsultants[0]] = true
		}
	default: // 12
		for _, id := range state.RecentLeaders {
			ineligible[id] = true
		}
		for _, id := range state.RecentConsultants {
			ineligible[id] = true
		}
	}

	// sequential rotation: find next eligible player after current leader
	nPlayers := len(state.Players)
	for i := 1; i <= nPlayers; i++ {
		candidate := state.Players[(currentIdx+i)%nPlayers]
		if !ineligible[candidate.ID] {
			return candidate.ID
		}
	}

	// fallback: next in rotation even if ineligible
	return state.Players[(currentIdx+1)%nPlayers].ID
}

func isConsultantEligible(targetID int64, state *GameState) bool {
	if targetID == state.LeaderID {
		return false
	}
	if len(state.RecentLeaders) > 0 && state.RecentLeaders[0] == targetID {
		return false
	}
	if len(state.RecentConsultants) > 0 && state.RecentConsultants[0] == targetID {
		return false
	}
	return true
}

func consultantEligiblePlayers(state *GameState) []Player {
	eligible := []Player{}
	for _, p := range state.Players {
		if p.ID != state.LeaderID && isConsultantEligible(p.ID, state) {
			eligible = append(eligible, p)
		}
	}
	return eligible
}

func (g *AlMuamara) updateRotationHistory(state *GameState) {
	if state.LeaderID != 0 {
		state.RecentLeaders = prependInt64(state.RecentLeaders, state.LeaderID, 2)
	}
	if state.ConsultantID != 0 {
		state.RecentConsultants = prependInt64(state.RecentConsultants, state.ConsultantID, 2)
	}
}

// ─── Broadcast helpers ────────────────────────────────────────────────────────

func (g *AlMuamara) broadcast(room *lobby.GameRoom, state *GameState) {
	room.Mu.RLock()
	defer room.Mu.RUnlock()
	for _, sess := range room.Participants {
		sess.Push("onMuamaraUpdate", state)
	}
	for _, sess := range room.Spectators {
		sess.Push("onMuamaraUpdate", state)
	}
}

func (g *AlMuamara) pushCardsToPlayer(s *session.Session, uid int64, state *GameState) {
	switch state.Phase {
	case phaseLeaderCard:
		if uid == state.LeaderID && len(state.drawnCards) > 0 {
			s.Push("onMuamaraCards", &CardsPayload{Cards: state.drawnCards, Phase: phaseLeaderCard})
		}
	case phaseConsultantCard:
		if uid == state.ConsultantID && len(state.drawnCards) > 0 {
			s.Push("onMuamaraCards", &CardsPayload{Cards: state.drawnCards, Phase: phaseConsultantCard})
		}
	}
}

// ─── Timer helpers ────────────────────────────────────────────────────────────

func (g *AlMuamara) scheduleTimer(roomID, key string, delayMs int, fn func()) {
	fullKey := roomID + ":" + key
	g.mu.Lock()
	if t, ok := g.timers[fullKey]; ok {
		t.Stop()
	}
	g.timers[fullKey] = time.AfterFunc(time.Duration(delayMs)*time.Millisecond, fn)
	g.mu.Unlock()
}

func (g *AlMuamara) cancelTimer(roomID string) {
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

// ─── Utility helpers ──────────────────────────────────────────────────────────

func findPlayer(players []Player, id int64) *Player {
	for i := range players {
		if players[i].ID == id {
			return &players[i]
		}
	}
	return nil
}

func removePlayerFromSlice(players []Player, id int64) []Player {
	for i, p := range players {
		if p.ID == id {
			return append(players[:i], players[i+1:]...)
		}
	}
	return players
}

func countAlivePlayers(state *GameState) int {
	return len(state.Players)
}

func prependInt64(slice []int64, val int64, maxLen int) []int64 {
	result := append([]int64{val}, slice...)
	if len(result) > maxLen {
		result = result[:maxLen]
	}
	return result
}

func filterInt64(slice []int64, remove int64) []int64 {
	filtered := make([]int64, 0, len(slice))
	for _, v := range slice {
		if v != remove {
			filtered = append(filtered, v)
		}
	}
	return filtered
}

func snapshotSessions(room *lobby.GameRoom) map[int64]*session.Session {
	m := make(map[int64]*session.Session, len(room.Participants))
	for id, s := range room.Participants {
		m[id] = s
	}
	return m
}

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

func votesMapToJSON(votes map[int64]bool) map[string]bool {
	m := make(map[string]bool, len(votes))
	for k, v := range votes {
		m[strconv.FormatInt(k, 10)] = v
	}
	return m
}

func votesMapFromJSON(m map[string]bool) map[int64]bool {
	result := make(map[int64]bool, len(m))
	for k, v := range m {
		id, _ := strconv.ParseInt(k, 10, 64)
		result[id] = v
	}
	return result
}

func orEmpty(s []string) []string {
	if s == nil {
		return []string{}
	}
	return s
}

func orUnknown(p *Player) string {
	if p == nil {
		return "Unknown"
	}
	return p.Username
}

// ─── dynamicJoinState interface ───────────────────────────────────────────────

func (s *GameState) MaxPlayers() int        { return MaxPlayers }
func (s *GameState) CanJoinDuringPlay() bool { return false }
func (s *GameState) AddParticipant(_ int64) bool { return false }
func (s *GameState) RemoveParticipant(_ int64)    {}

// LastVoteApproved is a helper field (not in JSON by default)
// We add it to the state struct above — but Go struct json ignores unexported fields.
// Since we declared it exported, it IS in the JSON. That's intentional for front-end.
