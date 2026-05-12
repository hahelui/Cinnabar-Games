package rps

import (
	"encoding/json"
	"math/rand"
	"time"

	"github.com/cinnabar-games/backend/internal/auth"
	"github.com/cinnabar-games/backend/internal/lobby"
	"github.com/cinnabar-games/backend/internal/protocol"
	"github.com/lonng/nano/component"
	"github.com/lonng/nano/session"
)

type RockPaperScissors struct {
	component.Base
	lobby *lobby.Lobby
}

type PlayReq struct {
	RoomID string `json:"room_id"`
	Move   string `json:"move"` // rock, paper, scissors
}

type GameStateMsg struct {
	Round      int            `json:"round"`
	MaxRounds  int            `json:"max_rounds"`
	Scores     map[int64]int  `json:"scores"`
	Moves      map[int64]string `json:"moves"`      // current round moves (hidden until both play)
	Revealed   bool           `json:"revealed"`
	Winner     int64          `json:"winner,omitempty"`
	Status     string         `json:"status"`
	Players    []int64        `json:"players"`
}

func NewRPS(lob *lobby.Lobby) *RockPaperScissors {
	return &RockPaperScissors{lobby: lob}
}

func BlankState() *GameState {
	return &GameState{
		Round:     1,
		MaxRounds: 3,
		Scores:    make(map[int64]int),
		Moves:     make(map[int64]string),
		Players:   []int64{},
		Status:    "playing",
	}
}

func (r *RockPaperScissors) Play(s *session.Session, req *PlayReq) error {
	uid, _, err := auth.BindPlayer(s)
	if err != nil {
		return s.Response(&protocol.Response{Code: 401, Message: err.Error()})
	}

	room, err := r.lobby.GetRoom(req.RoomID)
	if err != nil {
		return s.Response(&protocol.Response{Code: 404, Message: err.Error()})
	}

	room.Mu.Lock()

	if room.Status != "playing" {
		room.Mu.Unlock()
		return s.Response(&protocol.Response{Code: 400, Message: "game not active"})
	}

	state, ok := room.GameData.(*GameState)
	if !ok {
		room.Mu.Unlock()
		return s.Response(&protocol.Response{Code: 500, Message: "state corrupted"})
	}

	isParticipant := false
	for _, pid := range state.Players {
		if pid == uid {
			isParticipant = true
			break
		}
	}
	if !isParticipant {
		room.Mu.Unlock()
		return s.Response(&protocol.Response{Code: 403, Message: "spectators cannot play"})
	}

	if req.Move != "rock" && req.Move != "paper" && req.Move != "scissors" {
		room.Mu.Unlock()
		return s.Response(&protocol.Response{Code: 400, Message: "invalid move"})
	}

	state.Moves[uid] = req.Move

	allMoved := true
	for _, pid := range state.Players {
		if _, ok := state.Moves[pid]; !ok {
			allMoved = false
			break
		}
	}

	if allMoved && len(state.Players) >= 2 {
		state.Revealed = true
		r.resolveRound(state)
		if state.Round >= state.MaxRounds {
			state.Status = "finished"
			room.Status = "finished"
		} else {
			state.Round++
			state.Moves = make(map[int64]string)
			state.Revealed = false
		}
	}

	msg := r.toMsg(state)
	room.Mu.Unlock()

	r.broadcastToRoom(room, "onRPSUpdate", msg)
	r.lobby.PersistRoomAndState(room)
	return s.Response(&protocol.Response{Code: 0})
}

func (r *RockPaperScissors) GetState(s *session.Session, req *PlayReq) error {
	room, err := r.lobby.GetRoom(req.RoomID)
	if err != nil {
		return s.Response(&protocol.Response{Code: 404, Message: err.Error()})
	}
	room.Mu.RLock()
	state, ok := room.GameData.(*GameState)
	room.Mu.RUnlock()
	if !ok {
		return s.Response(&protocol.Response{Code: 500, Message: "no state"})
	}
	return s.Response(r.toMsg(state))
}

func (r *RockPaperScissors) InitGame(room *lobby.GameRoom) error {
	room.Mu.Lock()
	var players []int64
	for pid := range room.Participants {
		players = append(players, pid)
	}
	if len(players) < 2 {
		room.Mu.Unlock()
		return nil // allow single-player practice mode
	}

	scores := make(map[int64]int)
	for _, pid := range players {
		scores[pid] = 0
	}

	state := &GameState{
		Round:     1,
		MaxRounds: 3,
		Scores:    scores,
		Moves:     make(map[int64]string),
		Players:   players,
		Status:    "playing",
	}
	room.GameData = state
	room.Mu.Unlock()

	msg := r.toMsg(state)
	r.broadcastToRoom(room, "onRPSUpdate", msg)
	r.lobby.PersistRoomAndState(room)
	return nil
}

func (r *RockPaperScissors) toMsg(s *GameState) *GameStateMsg {
	return &GameStateMsg{
		Round:     s.Round,
		MaxRounds: s.MaxRounds,
		Scores:    s.Scores,
		Moves:     s.Moves,
		Revealed:  s.Revealed,
		Winner:    s.Winner,
		Status:    s.Status,
		Players:   s.Players,
	}
}

func (r *RockPaperScissors) broadcastToRoom(room *lobby.GameRoom, route string, v interface{}) {
	room.Mu.RLock()
	defer room.Mu.RUnlock()
	for _, sess := range room.Participants {
		sess.Push(route, v)
	}
	for _, sess := range room.Spectators {
		sess.Push(route, v)
	}
}

func (r *RockPaperScissors) resolveRound(state *GameState) {
	if len(state.Players) != 2 {
		return
	}
	p1 := state.Players[0]
	p2 := state.Players[1]
	m1 := state.Moves[p1]
	m2 := state.Moves[p2]

	if m1 == m2 {
		// draw
		return
	}

	beats := map[string]string{
		"rock":     "scissors",
		"paper":    "rock",
		"scissors": "paper",
	}

	if beats[m1] == m2 {
		state.Scores[p1]++
	} else {
		state.Scores[p2]++
	}

	// Check overall winner
	if state.Round >= state.MaxRounds {
		if state.Scores[p1] > state.Scores[p2] {
			state.Winner = p1
		} else if state.Scores[p2] > state.Scores[p1] {
			state.Winner = p2
		} else {
			state.Winner = 0 // draw
		}
	}
}

type GameState struct {
	Round     int
	MaxRounds int
	Scores    map[int64]int
	Moves     map[int64]string
	Revealed  bool
	Winner    int64
	Players   []int64
	Status    string
}

func (s *GameState) GetStatus() string { return s.Status }


func (s *GameState) MarshalState() ([]byte, error) {
	msg := GameStateMsg{
		Round:     s.Round,
		MaxRounds: s.MaxRounds,
		Scores:    s.Scores,
		Moves:     s.Moves,
		Revealed:  s.Revealed,
		Winner:    s.Winner,
		Status:    s.Status,
		Players:   s.Players,
	}
	return json.Marshal(msg)
}

func init() {
	rand.Seed(time.Now().UnixNano())
}

func (s *GameState) MaxPlayers() int {
	return 2
}

func (s *GameState) CanJoinDuringPlay() bool {
	if s.Status != "playing" {
		return true
	}
	return len(s.Players) < 2
}

func (s *GameState) AddParticipant(playerID int64) bool {
	for _, pid := range s.Players {
		if pid == playerID {
			if _, ok := s.Scores[playerID]; !ok {
				s.Scores[playerID] = 0
			}
			return true
		}
	}
	if len(s.Players) >= 2 {
		return false
	}
	s.Players = append(s.Players, playerID)
	if s.Scores == nil {
		s.Scores = make(map[int64]int)
	}
	if s.Moves == nil {
		s.Moves = make(map[int64]string)
	}
	s.Scores[playerID] = 0
	return true
}

func (s *GameState) RemoveParticipant(playerID int64) {
	idx := -1
	for i, pid := range s.Players {
		if pid == playerID {
			idx = i
			break
		}
	}
	if idx != -1 {
		s.Players = append(s.Players[:idx], s.Players[idx+1:]...)
	}
	delete(s.Moves, playerID)
	delete(s.Scores, playerID)
}
