package roulette

import (
	"encoding/json"
	"fmt"
	"math/rand"
	"sync"
	"time"

	"github.com/cinnabar-games/backend/internal/auth"
	"github.com/cinnabar-games/backend/internal/lobby"
	"github.com/cinnabar-games/backend/internal/protocol"
	"github.com/lonng/nano/component"
	"github.com/lonng/nano/session"
)

const (
	MinPlayers = 3
	MaxPlayers = 20

	countdownMs = 3000
	spinMs      = 5500
	decideMs    = 30000
	resultMs    = 2500

	phaseCountdown = "countdown"
	phaseSpinning  = "spinning"
	phaseDeciding  = "deciding"
	phaseResult    = "result"
	phaseFinished  = "finished"
)

type Roulette struct {
	component.Base
	lobby *lobby.Lobby
	mu    sync.Mutex
	timers map[string]*time.Timer
}

type Player struct {
	ID       int64  `json:"id"`
	Username string `json:"username"`
}

type EliminationEntry struct {
	PlayerID int64  `json:"player_id"`
	Username string `json:"username"`
	Round    int    `json:"round"`
	Reason   string `json:"reason"` // "kicked" | "withdrew" | "timeout"
	By       string `json:"by,omitempty"`
}

type GameState struct {
	Players          []Player           `json:"players"`
	Eliminated       []EliminationEntry `json:"eliminated"`
	Phase            string             `json:"phase"`
	Round            int                `json:"round"`
	SelectedID       int64              `json:"selected_id,omitempty"`
	SelectedIndex    int                `json:"selected_index"`
	SpinAngle        float64            `json:"spin_angle"`
	SpinDurationMs   int                `json:"spin_duration_ms"`
	PhaseEndsAt      int64              `json:"phase_ends_at"`
	DecisionDeadline int64              `json:"decision_deadline,omitempty"`
	LastEvent        string             `json:"last_event"`
	Winner           int64              `json:"winner,omitempty"`
	WinnerName       string             `json:"winner_name,omitempty"`
	Status           string             `json:"status"`

	totalRotation float64
}

func (s *GameState) GetStatus() string { return s.Status }


func (s *GameState) MarshalState() ([]byte, error) {
	// Use a temporary struct to include the unexported field
	type Alias GameState
	out := &struct {
		TotalRotation float64 `json:"total_rotation"`
		*Alias
	}{
		TotalRotation: s.totalRotation,
		Alias:         (*Alias)(s),
	}
	return json.Marshal(out)
}

type ChooseReq struct {
	RoomID   string `json:"room_id"`
	Action   string `json:"action"`             // "kick" | "withdraw"
	TargetID int64  `json:"target_id,omitempty"` // required for kick
}

type RoomReq struct {
	RoomID string `json:"room_id"`
}

func NewRoulette(lob *lobby.Lobby) *Roulette {
	return &Roulette{
		lobby:  lob,
		timers: make(map[string]*time.Timer),
	}
}

func BlankState() *GameState {
	return &GameState{
		Players:    []Player{},
		Eliminated: []EliminationEntry{},
		Phase:      phaseCountdown,
		Round:      0,
		Status:     "playing",
	}
}

// InitGame is called by lobby when host starts the game.
func (r *Roulette) InitGame(room *lobby.GameRoom) error {
	room.Mu.Lock()
	if len(room.Participants) < MinPlayers {
		room.Mu.Unlock()
		return fmt.Errorf("need at least %d players", MinPlayers)
	}

	players := make([]Player, 0, len(room.Participants))
	for pid, sess := range room.Participants {
		players = append(players, Player{ID: pid, Username: sess.String("username")})
	}
	// Shuffle so order on the wheel is randomized
	rand.Shuffle(len(players), func(i, j int) { players[i], players[j] = players[j], players[i] })

	state := &GameState{
		Players:    players,
		Eliminated: []EliminationEntry{},
		Phase:      phaseCountdown,
		Round:      0,
		Status:     "playing",
		LastEvent:  "Game starting — wheel spins in 3...",
	}
	state.PhaseEndsAt = time.Now().Add(countdownMs * time.Millisecond).UnixMilli()
	room.GameData = state
	roomID := room.ID
	room.Mu.Unlock()

	r.broadcast(room, state)
	r.lobby.PersistRoomAndState(room)
	r.scheduleAdvance(roomID, countdownMs)
	return nil
}

func (r *Roulette) RestoreGame(room *lobby.GameRoom) {
	room.Mu.Lock()
	state, ok := room.GameData.(*GameState)
	if !ok {
		room.Mu.Unlock()
		return
	}
	roomID := room.ID
	phase := state.Phase
	endsIn := state.PhaseEndsAt - time.Now().UnixMilli()
	room.Mu.Unlock()

	if state.Status == "finished" {
		return
	}

	if endsIn < 0 {
		endsIn = 0
	}

	switch phase {
	case phaseCountdown, phaseSpinning, phaseDeciding, phaseResult:
		r.scheduleAdvance(roomID, int(endsIn))
	}
}

// Choose handler: selected player chooses to kick another or withdraw.
func (r *Roulette) Choose(s *session.Session, req *ChooseReq) error {
	uid, _, err := auth.BindPlayer(s)
	if err != nil {
		return s.Response(&protocol.Response{Code: 401, Message: err.Error()})
	}
	room, err := r.lobby.GetRoom(req.RoomID)
	if err != nil {
		return s.Response(&protocol.Response{Code: 404, Message: err.Error()})
	}

	room.Mu.Lock()
	state, ok := room.GameData.(*GameState)
	if !ok || state.Status != "playing" {
		room.Mu.Unlock()
		return s.Response(&protocol.Response{Code: 400, Message: "game not active"})
	}
	if state.Phase != phaseDeciding {
		room.Mu.Unlock()
		return s.Response(&protocol.Response{Code: 400, Message: "not decision phase"})
	}
	if state.SelectedID != uid {
		room.Mu.Unlock()
		return s.Response(&protocol.Response{Code: 403, Message: "not your turn"})
	}

	selected := findPlayer(state.Players, uid)
	if selected == nil {
		room.Mu.Unlock()
		return s.Response(&protocol.Response{Code: 500, Message: "selected player missing"})
	}

	switch req.Action {
	case "withdraw":
		removePlayer(state, uid)
		state.Eliminated = append(state.Eliminated, EliminationEntry{
			PlayerID: selected.ID, Username: selected.Username, Round: state.Round, Reason: "withdrew",
		})
		state.LastEvent = fmt.Sprintf("%s has withdrawn from the wheel", selected.Username)
	case "kick":
		if req.TargetID == uid {
			room.Mu.Unlock()
			return s.Response(&protocol.Response{Code: 400, Message: "cannot kick yourself"})
		}
		target := findPlayer(state.Players, req.TargetID)
		if target == nil {
			room.Mu.Unlock()
			return s.Response(&protocol.Response{Code: 400, Message: "target not on wheel"})
		}
		removePlayer(state, target.ID)
		state.Eliminated = append(state.Eliminated, EliminationEntry{
			PlayerID: target.ID, Username: target.Username, Round: state.Round, Reason: "kicked", By: selected.Username,
		})
		state.LastEvent = fmt.Sprintf("%s kicked %s off the wheel", selected.Username, target.Username)
	default:
		room.Mu.Unlock()
		return s.Response(&protocol.Response{Code: 400, Message: "invalid action"})
	}

	r.cancelTimer(room.ID)
	r.afterDecision(state, room)
	roomID := room.ID
	phase := state.Phase
	endsIn := state.PhaseEndsAt - time.Now().UnixMilli()
	if endsIn < 0 {
		endsIn = 0
	}
	room.Mu.Unlock()

	r.broadcast(room, state)
	r.lobby.PersistRoomAndState(room)
	if phase != phaseFinished {
		r.scheduleAdvance(roomID, int(endsIn))
	} else {
		room.Mu.Lock()
		room.Status = "finished"
		room.Mu.Unlock()
	}
	return s.Response(&protocol.Response{Code: 0})
}

// GetState handler for clients reconnecting / opening the page.
func (r *Roulette) GetState(s *session.Session, req *RoomReq) error {
	room, err := r.lobby.GetRoom(req.RoomID)
	if err != nil {
		return s.Response(&protocol.Response{Code: 404, Message: err.Error()})
	}
	room.Mu.RLock()
	state, ok := room.GameData.(*GameState)
	room.Mu.RUnlock()
	if !ok {
		return s.Response(&protocol.Response{Code: 404, Message: "no state"})
	}
	return s.Response(state)
}

// --- Phase machine ---

func (r *Roulette) advance(roomID string) {
	room, err := r.lobby.GetRoom(roomID)
	if err != nil {
		return
	}
	room.Mu.Lock()
	state, ok := room.GameData.(*GameState)
	if !ok || state.Status == "finished" {
		room.Mu.Unlock()
		return
	}

	switch state.Phase {
	case phaseCountdown:
		r.startSpin(state)
	case phaseSpinning:
		r.startDeciding(state)
	case phaseDeciding:
		// timeout: wheel kicks the selected player
		sel := findPlayer(state.Players, state.SelectedID)
		if sel != nil {
			removePlayer(state, sel.ID)
			state.Eliminated = append(state.Eliminated, EliminationEntry{
				PlayerID: sel.ID, Username: sel.Username, Round: state.Round, Reason: "timeout",
			})
			state.LastEvent = fmt.Sprintf("Time's up — the wheel kicked %s", sel.Username)
		}
		r.afterDecision(state, room)
	case phaseResult:
		r.startCountdown(state)
	}

	endsIn := state.PhaseEndsAt - time.Now().UnixMilli()
	if endsIn < 0 {
		endsIn = 0
	}
	phase := state.Phase
	room.Mu.Unlock()

	r.broadcast(room, state)
	r.lobby.PersistRoomAndState(room)
	if phase != phaseFinished {
		r.scheduleAdvance(roomID, int(endsIn))
	} else {
		room.Mu.Lock()
		room.Status = "finished"
		room.Mu.Unlock()
	}
}

func (r *Roulette) startCountdown(state *GameState) {
	state.Phase = phaseCountdown
	state.SelectedID = 0
	state.PhaseEndsAt = time.Now().Add(countdownMs * time.Millisecond).UnixMilli()
	state.LastEvent = "Wheel spins in 3..."
}

func (r *Roulette) startSpin(state *GameState) {
	state.Round++
	n := len(state.Players)
	idx := rand.Intn(n)
	state.SelectedIndex = idx
	state.SelectedID = state.Players[idx].ID

	sliceAngle := 360.0 / float64(n)
	off := (rand.Float64() - 0.5) * sliceAngle * 0.8
	full := 4.0 + rand.Float64()*3.0
	delta := full*360.0 + (360.0-(float64(idx)*sliceAngle+sliceAngle/2.0)) + off
	state.totalRotation += delta
	state.SpinAngle = state.totalRotation
	state.SpinDurationMs = spinMs
	state.PhaseEndsAt = time.Now().Add(spinMs*time.Millisecond).UnixMilli()
	state.Phase = phaseSpinning
	state.LastEvent = "Wheel is spinning..."
}

func (r *Roulette) startDeciding(state *GameState) {
	state.Phase = phaseDeciding
	state.PhaseEndsAt = time.Now().Add(decideMs*time.Millisecond).UnixMilli()
	state.DecisionDeadline = state.PhaseEndsAt
	sel := findPlayer(state.Players, state.SelectedID)
	if sel != nil {
		state.LastEvent = fmt.Sprintf("Wheel chose %s — kick someone or withdraw", sel.Username)
	}
}

func (r *Roulette) afterDecision(state *GameState, _ *lobby.GameRoom) {
	if len(state.Players) <= 1 {
		state.Phase = phaseFinished
		state.Status = "finished"
		state.PhaseEndsAt = time.Now().UnixMilli()
		state.SelectedID = 0
		state.DecisionDeadline = 0
		if len(state.Players) == 1 {
			state.Winner = state.Players[0].ID
			state.WinnerName = state.Players[0].Username
			state.LastEvent = fmt.Sprintf("%s wins the roulette!", state.Players[0].Username)
		} else {
			state.LastEvent = "No winner — wheel is empty"
		}
		return
	}
	state.Phase = phaseResult
	state.SelectedID = 0
	state.DecisionDeadline = 0
	state.PhaseEndsAt = time.Now().Add(resultMs*time.Millisecond).UnixMilli()
}

// --- Helpers ---

func findPlayer(players []Player, id int64) *Player {
	for i := range players {
		if players[i].ID == id {
			return &players[i]
		}
	}
	return nil
}

func removePlayer(state *GameState, id int64) {
	for i, p := range state.Players {
		if p.ID == id {
			state.Players = append(state.Players[:i], state.Players[i+1:]...)
			return
		}
	}
}

func (r *Roulette) scheduleAdvance(roomID string, delayMs int) {
	r.cancelTimer(roomID)
	if delayMs < 0 {
		delayMs = 0
	}
	r.mu.Lock()
	r.timers[roomID] = time.AfterFunc(time.Duration(delayMs)*time.Millisecond, func() {
		r.advance(roomID)
	})
	r.mu.Unlock()
}

func (r *Roulette) cancelTimer(roomID string) {
	r.mu.Lock()
	if t, ok := r.timers[roomID]; ok {
		t.Stop()
		delete(r.timers, roomID)
	}
	r.mu.Unlock()
}

func (r *Roulette) broadcast(room *lobby.GameRoom, state *GameState) {
	room.Mu.RLock()
	defer room.Mu.RUnlock()
	for _, sess := range room.Participants {
		sess.Push("onRouletteUpdate", state)
	}
	for _, sess := range room.Spectators {
		sess.Push("onRouletteUpdate", state)
	}
}

// --- dynamicJoinState interface for lobby ---

func (s *GameState) MaxPlayers() int { return MaxPlayers }

func (s *GameState) CanJoinDuringPlay() bool { return false }

func (s *GameState) AddParticipant(_ int64) bool { return false }

func (s *GameState) RemoveParticipant(playerID int64) {
	// If a participant disconnects mid-game, treat as withdrawal.
	for _, p := range s.Players {
		if p.ID == playerID {
			s.Eliminated = append(s.Eliminated, EliminationEntry{
				PlayerID: p.ID, Username: p.Username, Round: s.Round, Reason: "withdrew",
			})
			break
		}
	}
	removePlayer(s, playerID)
}
