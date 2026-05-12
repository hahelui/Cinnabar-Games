package tictactoe

import (
	"encoding/json"
	"fmt"
	"strconv"
	"strings"

	"github.com/cinnabar-games/backend/internal/auth"
	"github.com/cinnabar-games/backend/internal/lobby"
	"github.com/cinnabar-games/backend/internal/protocol"
	"github.com/lonng/nano/component"
	"github.com/lonng/nano/session"
)

type TicTacToe struct {
	component.Base
	lobby *lobby.Lobby
}

type MakeMoveReq struct {
	RoomID string `json:"room_id"`
	Index  int    `json:"index"` // 0-8
}

type GameStateMsg struct {
	Board      [9]string `json:"board"` // "", "X", "O"
	Turn       int64     `json:"turn"`  // playerID whose turn it is
	Winner     int64     `json:"winner,omitempty"`
	IsDraw     bool      `json:"is_draw,omitempty"`
	Players    []int64   `json:"players"` // [X player, O player]
	PlayerX    int64     `json:"player_x"`
	PlayerO    int64     `json:"player_o"`
	Status     string    `json:"status"` // waiting | playing | finished
}

func NewTicTacToe(lob *lobby.Lobby) *TicTacToe {
	return &TicTacToe{lobby: lob}
}

func BlankState() *GameState {
	return &GameState{
		Board:   [9]string{},
		Status:  "playing",
		Players: []int64{},
	}
}

func (t *TicTacToe) MakeMove(s *session.Session, req *MakeMoveReq) error {
	uid, _, err := auth.BindPlayer(s)
	if err != nil {
		return s.Response(&protocol.Response{Code: 401, Message: err.Error()})
	}

	room, err := t.lobby.GetRoom(req.RoomID)
	if err != nil {
		return s.Response(&protocol.Response{Code: 404, Message: err.Error()})
	}

	room.Mu.Lock()

	if room.Status != "playing" {
		room.Mu.Unlock()
		return s.Response(&protocol.Response{Code: 400, Message: "game not started"})
	}

	state, ok := room.GameData.(*GameState)
	if !ok {
		room.Mu.Unlock()
		return s.Response(&protocol.Response{Code: 500, Message: "game state corrupted"})
	}

	if uid != state.PlayerX && uid != state.PlayerO {
		room.Mu.Unlock()
		return s.Response(&protocol.Response{Code: 403, Message: "spectators cannot move"})
	}

	if req.Index < 0 || req.Index > 8 {
		room.Mu.Unlock()
		return s.Response(&protocol.Response{Code: 400, Message: "invalid index"})
	}
	if state.Board[req.Index] != "" {
		room.Mu.Unlock()
		return s.Response(&protocol.Response{Code: 400, Message: "cell occupied"})
	}
	if state.Turn != uid {
		room.Mu.Unlock()
		return s.Response(&protocol.Response{Code: 400, Message: "not your turn"})
	}

	mark := "X"
	if uid == state.PlayerO {
		mark = "O"
	}
	state.Board[req.Index] = mark

	if winner := checkWin(state.Board); winner != "" {
		if winner == "X" {
			state.Winner = state.PlayerX
		} else {
			state.Winner = state.PlayerO
		}
		state.Status = "finished"
		room.Status = "finished"
	} else if isDraw(state.Board) {
		state.IsDraw = true
		state.Status = "finished"
		room.Status = "finished"
	} else {
		// Switch turn
		if state.Turn == state.PlayerX {
			state.Turn = state.PlayerO
		} else {
			state.Turn = state.PlayerX
		}
	}

	msg := t.toMsg(state)
	room.Mu.Unlock()

	t.broadcastToRoom(room, "onTicTacToeUpdate", msg)
	t.lobby.PersistRoomAndState(room)
	return s.Response(&protocol.Response{Code: 0})
}

func (t *TicTacToe) GetState(s *session.Session, req *MakeMoveReq) error {
	room, err := t.lobby.GetRoom(req.RoomID)
	if err != nil {
		return s.Response(&protocol.Response{Code: 404, Message: err.Error()})
	}
	room.Mu.RLock()
	state, ok := room.GameData.(*GameState)
	room.Mu.RUnlock()
	if !ok {
		return s.Response(&protocol.Response{Code: 500, Message: "no state"})
	}
	return s.Response(t.toMsg(state))
}

// InitGame is called by lobby when game starts
func (t *TicTacToe) InitGame(room *lobby.GameRoom) error {
	room.Mu.Lock()
	var players []int64
	for pid := range room.Participants {
		players = append(players, pid)
	}
	if len(players) < 2 {
		room.Mu.Unlock()
		return fmt.Errorf("need 2 players")
	}

	state := &GameState{
		Board:   [9]string{},
		Turn:    players[0],
		PlayerX: players[0],
		PlayerO: players[1],
		Players: players,
		Status:  "playing",
	}
	room.GameData = state
	room.Mu.Unlock()

	msg := t.toMsg(state)
	t.broadcastToRoom(room, "onTicTacToeUpdate", msg)
	t.lobby.PersistRoomAndState(room)
	return nil
}

func (t *TicTacToe) toMsg(s *GameState) *GameStateMsg {
	return &GameStateMsg{
		Board:   s.Board,
		Turn:    s.Turn,
		Winner:  s.Winner,
		IsDraw:  s.IsDraw,
		Players: s.Players,
		PlayerX: s.PlayerX,
		PlayerO: s.PlayerO,
		Status:  s.Status,
	}
}

func (t *TicTacToe) broadcastToRoom(r *lobby.GameRoom, route string, v interface{}) {
	r.Mu.RLock()
	defer r.Mu.RUnlock()
	for _, sess := range r.Participants {
		sess.Push(route, v)
	}
	for _, sess := range r.Spectators {
		sess.Push(route, v)
	}
}

type GameState struct {
	Board   [9]string
	Turn    int64
	Winner  int64
	IsDraw  bool
	Players []int64
	PlayerX int64
	PlayerO int64
	Status  string
}

func (s *GameState) GetStatus() string { return s.Status }


func (s *GameState) MarshalState() ([]byte, error) {
	msg := GameStateMsg{
		Board:   s.Board,
		Turn:    s.Turn,
		Winner:  s.Winner,
		IsDraw:  s.IsDraw,
		Players: s.Players,
		PlayerX: s.PlayerX,
		PlayerO: s.PlayerO,
		Status:  s.Status,
	}
	return json.Marshal(msg)
}

func checkWin(b [9]string) string {
	lines := [][3]int{
		{0, 1, 2}, {3, 4, 5}, {6, 7, 8},
		{0, 3, 6}, {1, 4, 7}, {2, 5, 8},
		{0, 4, 8}, {2, 4, 6},
	}
	for _, l := range lines {
		if b[l[0]] != "" && b[l[0]] == b[l[1]] && b[l[1]] == b[l[2]] {
			return b[l[0]]
		}
	}
	return ""
}

func isDraw(b [9]string) bool {
	for _, v := range b {
		if v == "" {
			return false
		}
	}
	return true
}

func (s *GameState) MaxPlayers() int {
	return 2
}

func (s *GameState) CanJoinDuringPlay() bool {
	if s.Status != "playing" {
		return true
	}
	if len(s.Players) < 2 {
		return true
	}
	if s.PlayerX == 0 || s.PlayerO == 0 {
		return true
	}
	return false
}

func (s *GameState) AddParticipant(playerID int64) bool {
	for _, pid := range s.Players {
		if pid == playerID {
			if s.PlayerX == 0 {
				s.PlayerX = playerID
				s.Turn = playerID
			}
			if s.PlayerO == 0 {
				s.PlayerO = playerID
			}
			return true
		}
	}
	if len(s.Players) >= 2 {
		return false
	}
	s.Players = append(s.Players, playerID)
	if s.PlayerX == 0 {
		s.PlayerX = playerID
		s.Turn = playerID
	} else if s.PlayerO == 0 {
		s.PlayerO = playerID
		if s.Turn == 0 {
			s.Turn = s.PlayerX
		}
	}
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
	if s.PlayerX == playerID {
		s.PlayerX = 0
	}
	if s.PlayerO == playerID {
		s.PlayerO = 0
	}
	if s.Turn == playerID {
		if s.PlayerX != 0 {
			s.Turn = s.PlayerX
		} else if s.PlayerO != 0 {
			s.Turn = s.PlayerO
		} else {
			s.Turn = 0
		}
	}
}

// ParseMove parses raw JSON bytes for quick client testing
func ParseMove(raw []byte) (*MakeMoveReq, error) {
	// simplistic parse: {"room_id":"...","index":N}
	str := string(raw)
	var req MakeMoveReq
	// extract room_id
	if idx := strings.Index(str, `"room_id"`); idx != -1 {
		start := strings.Index(str[idx:], `"`) + idx + 1
		start = strings.Index(str[start:], `"`) + start + 1
		end := strings.Index(str[start:], `"`)
		if end != -1 {
			req.RoomID = str[start : start+end]
		}
	}
	if idx := strings.Index(str, `"index"`); idx != -1 {
		start := strings.Index(str[idx:], `:`) + idx + 1
		end := start + 1
		for end < len(str) && (str[end] >= '0' && str[end] <= '9' || str[end] == '-') {
			end++
		}
		if v, err := strconv.Atoi(strings.TrimSpace(str[start:end])); err == nil {
			req.Index = v
		}
	}
	return &req, nil
}
