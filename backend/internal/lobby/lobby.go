package lobby

import (
	"crypto/rand"
	"encoding/json"
	"fmt"
	"math/big"
	"strings"
	"sync"
	"time"

	"github.com/cinnabar-games/backend/internal/auth"
	"github.com/cinnabar-games/backend/internal/db"
	"github.com/cinnabar-games/backend/internal/protocol"
	"github.com/lonng/nano/component"
	"github.com/lonng/nano/session"
)

// playerInRoom returns the roomID a player is currently in, or "" if none.
func (l *Lobby) playerInRoom(uid int64) string {
	l.mu.RLock()
	defer l.mu.RUnlock()
	return l.playerRooms[uid]
}

type GameInitFunc func(*GameRoom) error

type PersistableState interface {
	MarshalState() ([]byte, error)
}

type dynamicJoinState interface {
	MaxPlayers() int
	CanJoinDuringPlay() bool
	AddParticipant(playerID int64) bool
	RemoveParticipant(playerID int64)
}

type Lobby struct {
	component.Base
	mu              sync.RWMutex
	rooms           map[string]*GameRoom
	gameIniter      map[string]GameInitFunc
	gameRestorer    map[string]func(*GameRoom)
	blankStates     map[string]func() interface{}
	playerRooms     map[int64]string // playerID → roomID for cross-room gating
	activeSessions  map[int64]*session.Session // latest session per uid
	RoomCreationKey string
}

type SavedPlayer struct {
	ID       int64
	Username string
	Role     string
}

type GameRoom struct {
	ID           string
	GameType     string
	HostID       int64
	Status       string
	Participants map[int64]*session.Session
	Spectators   map[int64]*session.Session
	SavedPlayers []SavedPlayer
	Config       string
	GameData     interface{}
	Mu           sync.RWMutex
	LastActivity time.Time
}

func (r *GameRoom) isKnownPlayer(uid int64) bool {
	for _, sp := range r.SavedPlayers {
		if sp.ID == uid && sp.Role == "player" {
			return true
		}
	}
	return false
}

func (r *GameRoom) isKnownSpectator(uid int64) bool {
	for _, sp := range r.SavedPlayers {
		if sp.ID == uid && sp.Role == "spectator" {
			return true
		}
	}
	return false
}

func NewLobby() *Lobby {
	l := &Lobby{
		rooms:          make(map[string]*GameRoom),
		gameIniter:     make(map[string]GameInitFunc),
		gameRestorer:   make(map[string]func(*GameRoom)),
		blankStates:    make(map[string]func() interface{}),
		playerRooms:    make(map[int64]string),
		activeSessions: make(map[int64]*session.Session),
	}

	// Register global session close handler to remove dead sessions from rooms
	session.Lifetime.OnClosed(func(s *session.Session) {
		defer func() {
			if r := recover(); r != nil {
				fmt.Printf("recovered from panic in session close handler: %v\n", r)
			}
		}()
		uid := s.UID()
		if uid == 0 {
			return
		}
		l.handleSessionDisconnect(uid, s)
	})

	return l
}

func (l *Lobby) RegisterActiveSession(uid int64, sess *session.Session) {
	l.mu.Lock()
	defer l.mu.Unlock()
	l.activeSessions[uid] = sess
}

func (l *Lobby) handleSessionDisconnect(uid int64, closingSess *session.Session) {
	l.mu.Lock()
	// Fast-path: if a newer session was already registered, skip entirely.
	if active, ok := l.activeSessions[uid]; ok && active != closingSess {
		l.mu.Unlock()
		return
	}
	delete(l.activeSessions, uid)

	roomID, ok := l.playerRooms[uid]
	if !ok {
		l.mu.Unlock()
		return
	}
	room, ok := l.rooms[roomID]
	if !ok {
		delete(l.playerRooms, uid)
		l.mu.Unlock()
		return
	}

	room.Mu.Lock()
	sess, inPlayers := room.Participants[uid]
	if inPlayers && sess != closingSess {
		room.Mu.Unlock()
		l.mu.Unlock()
		return
	}
	_, inSpectators := room.Spectators[uid]
	if !inPlayers && !inSpectators {
		room.Mu.Unlock()
		l.mu.Unlock()
		return
	}

	// Clear the dead session from live maps
	delete(room.Participants, uid)
	delete(room.Spectators, uid)

	// Known players keep their game state and host status on disconnect —
	// they may be refreshing the page (new session will reconnect via JoinRoom).
	// Only remove session references; don't call RemoveParticipant or transfer host.
	if room.isKnownPlayer(uid) || room.isKnownSpectator(uid) {
		room.LastActivity = time.Now()
		room.Mu.Unlock()
		// Keep playerRooms[uid] so JoinRoom finds them in the right room
		l.mu.Unlock()

		info := l.toRoomInfo(room)
		l.broadcastToRoom(room, "onRoomUpdated", info)
		l.saveParticipants(room)
		return
	}

	// Non-known: immediate full removal
	if gs, ok := room.GameData.(dynamicJoinState); ok {
		gs.RemoveParticipant(uid)
		checkGameFinished(room)
	}

	if room.HostID == uid {
		for _, sp := range room.SavedPlayers {
			if sp.ID != uid && sp.Role == "player" {
				room.HostID = sp.ID
				break
			}
		}
	}

	room.LastActivity = time.Now()
	room.Mu.Unlock()
	// Keep playerRooms[uid] for cross-room gating
	l.mu.Unlock()

	info := l.toRoomInfo(room)
	l.broadcastToRoom(room, "onRoomUpdated", info)
	l.saveParticipants(room)
}

// maybeLeaveCurrentRoom checks if the player is already in a different room
// and leaves it. Returns an error only if the server fails. Must NOT be called
// while holding l.mu or room.Mu.
func (l *Lobby) maybeLeaveCurrentRoom(uid int64, targetRoomID string) error {
	current := l.playerInRoom(uid)
	if current == "" || current == targetRoomID {
		return nil
	}
	l.leaveRoomForPlayer(uid)
	return nil
}

// leaveRoomForPlayer removes a player from their current room (if any).
// Must NOT be called while holding l.mu or room.Mu.
func (l *Lobby) leaveRoomForPlayer(uid int64) {
	l.mu.Lock()
	oldRoomID, ok := l.playerRooms[uid]
	if !ok {
		l.mu.Unlock()
		return
	}

	oldRoom, ok := l.rooms[oldRoomID]
	if !ok {
		delete(l.playerRooms, uid)
		l.mu.Unlock()
		return
	}

	oldRoom.Mu.Lock()
	delete(oldRoom.Participants, uid)
	delete(oldRoom.Spectators, uid)
	if gs, ok := oldRoom.GameData.(dynamicJoinState); ok {
		gs.RemoveParticipant(uid)
		checkGameFinished(oldRoom)
	}
	for i, sp := range oldRoom.SavedPlayers {
		if sp.ID == uid {
			oldRoom.SavedPlayers = append(oldRoom.SavedPlayers[:i], oldRoom.SavedPlayers[i+1:]...)
			break
		}
	}
	if uid == oldRoom.HostID {
		for _, sp := range oldRoom.SavedPlayers {
			if sp.ID != uid {
				oldRoom.HostID = sp.ID
				break
			}
		}
	}
	oldRoom.LastActivity = time.Now()
	shouldDelete := len(oldRoom.SavedPlayers) == 0
	oldRoom.Mu.Unlock()

	delete(l.playerRooms, uid)
	if shouldDelete {
		delete(l.rooms, oldRoomID)
	}
	l.mu.Unlock()

	if shouldDelete {
		db.DeleteRoomAndState(oldRoomID)
	} else {
		info := l.toRoomInfo(oldRoom)
		l.broadcastToRoom(oldRoom, "onRoomUpdated", info)
		l.PersistRoomAndState(oldRoom)
	}
}

func (l *Lobby) StartCleanup() {
	go func() {
		for {
			time.Sleep(30 * time.Second)
			l.cleanupStaleRooms()
		}
	}()
}

func (l *Lobby) cleanupStaleRooms() {
	l.mu.Lock()
	defer l.mu.Unlock()

	now := time.Now()
	toDelete := make([]string, 0)

	for id, room := range l.rooms {
		room.Mu.Lock()
		activeSessions := len(room.Participants) + len(room.Spectators)
		idle := now.Sub(room.LastActivity)
		hasSaved := len(room.SavedPlayers) > 0

		var ttl time.Duration
		switch room.Status {
		case "waiting":
			if hasSaved {
				ttl = 2 * time.Hour
			} else {
				ttl = 10 * time.Minute
			}
		case "playing":
			if hasSaved {
				ttl = 2 * time.Hour
			} else {
				ttl = 30 * time.Minute
			}
		case "finished":
			ttl = 5 * time.Minute
		default:
			ttl = 30 * time.Minute
		}

		if activeSessions > 0 {
			room.LastActivity = now
		}

		if idle > ttl && activeSessions == 0 {
			fmt.Printf("cleanup: deleting stale room %s (status=%s, idle=%v, saved=%d)\n", id, room.Status, idle.Round(time.Second), len(room.SavedPlayers))
			toDelete = append(toDelete, id)
		}
		room.Mu.Unlock()
	}

	// Also clean up playerRooms entries for the deleted rooms
	for _, id := range toDelete {
		for pid, rid := range l.playerRooms {
			if rid == id {
				delete(l.playerRooms, pid)
			}
		}
		delete(l.rooms, id)
		go db.DeleteRoomAndState(id)
	}
}

func (l *Lobby) RegisterGame(gameType string, initer GameInitFunc) {
	l.mu.Lock()
	defer l.mu.Unlock()
	l.gameIniter[gameType] = initer
}

func (l *Lobby) RegisterRestorer(gameType string, restorer func(*GameRoom)) {
	l.mu.Lock()
	defer l.mu.Unlock()
	l.gameRestorer[gameType] = restorer
}

func (l *Lobby) RegisterBlankState(gameType string, fn func() interface{}) {
	l.mu.Lock()
	defer l.mu.Unlock()
	l.blankStates[gameType] = fn
}

func (l *Lobby) createBlankState(gameType string) interface{} {
	if fn, ok := l.blankStates[gameType]; ok {
		return fn()
	}
	return nil
}

func (l *Lobby) LoadFromDB() {
	var rooms []db.Room
	if err := db.Instance.Where("status IN ?", []string{"playing", "finished"}).Find(&rooms).Error; err != nil {
		fmt.Printf("warn: failed to load rooms from db: %v\n", err)
		return
	}

	for _, roomRow := range rooms {
		var gs db.GameState
		if err := db.Instance.Where("room_id = ?", roomRow.ID).First(&gs).Error; err != nil {
			fmt.Printf("warn: no saved state for room %s, skipping\n", roomRow.ID)
			continue
		}

room := &GameRoom{
		ID:           roomRow.ID,
		GameType:     roomRow.GameType,
		HostID:       roomRow.HostID,
		Status:       roomRow.Status,
		Participants: make(map[int64]*session.Session),
		Spectators:   make(map[int64]*session.Session),
		Config:       roomRow.Config,
		LastActivity: time.Now(),
	}

	initer, ok := l.gameIniter[roomRow.GameType]
	if !ok {
		fmt.Printf("warn: no initer for game type %s, deleting room %s\n", roomRow.GameType, roomRow.ID)
		db.DeleteRoomAndState(roomRow.ID)
		continue
	}

	// Call InitGame to create the empty state struct, but pass empty participants
	// since sessions are dead on restart. The saved state will overwrite this.
	if err := initer(room); err != nil {
		// InitGame may fail due to min player checks with empty room — that's expected on restore.
		// Create a minimal blank state so we can unmarshal into it.
		fmt.Printf("info: init room %s failed (expected on restore with no sessions): %v, creating blank state\n", roomRow.ID, err)
		room.Mu.Lock()
		room.GameData = l.createBlankState(roomRow.GameType)
		room.Mu.Unlock()
		if room.GameData == nil {
			fmt.Printf("warn: could not create blank state for %s, deleting\n", roomRow.ID)
			db.DeleteRoomAndState(roomRow.ID)
			continue
		}
	}

	if room.GameData == nil {
		fmt.Printf("warn: initer left GameData nil for %s, deleting\n", roomRow.ID)
		db.DeleteRoomAndState(roomRow.ID)
		continue
	}

	if ps, ok := room.GameData.(PersistableState); ok {
		if err := json.Unmarshal([]byte(gs.StateJSON), ps); err != nil {
			fmt.Printf("warn: failed to unmarshal state for room %s: %v, deleting\n", roomRow.ID, err)
			db.DeleteRoomAndState(roomRow.ID)
			continue
		}
	} else {
		if err := json.Unmarshal([]byte(gs.StateJSON), room.GameData); err != nil {
			fmt.Printf("warn: failed to unmarshal state for room %s: %v, deleting\n", roomRow.ID, err)
			db.DeleteRoomAndState(roomRow.ID)
			continue
		}
	}

	room.Mu.Lock()
	room.Status = roomRow.Status
	room.Mu.Unlock()

		// Restore saved player list (sessions are dead, but we keep the metadata)
		savedPlayers, err := db.LoadParticipants(roomRow.ID)
		if err != nil {
			fmt.Printf("warn: failed to load participants for room %s: %v\n", roomRow.ID, err)
		} else {
			room.SavedPlayers = make([]SavedPlayer, 0, len(savedPlayers))
			for _, sp := range savedPlayers {
				room.SavedPlayers = append(room.SavedPlayers, SavedPlayer{
					ID:       sp.PlayerID,
					Username: sp.Username,
					Role:     sp.Role,
				})
			}
		}

		if restorer, ok := l.gameRestorer[roomRow.GameType]; ok {
			restorer(room)
		}

		l.mu.Lock()
		l.rooms[roomRow.ID] = room
		l.mu.Unlock()

		fmt.Printf("restored room %s (%s, status=%s, players=%d)\n", roomRow.ID, roomRow.GameType, roomRow.Status, len(room.SavedPlayers))
	}

	// Clean up rooms that have been around too long without anyone (stale)
	var allRooms []db.Room
	db.Instance.Find(&allRooms)
	for _, r := range allRooms {
		if _, exists := l.rooms[r.ID]; !exists {
			db.DeleteRoomAndState(r.ID)
		}
	}
}

func defaultMaxPlayers(gameType string) int {
	switch gameType {
	case "tictactoe", "rps":
		return 2
	case "roulette":
		return 20
	case "almuamara":
		return 12
	case "mafia":
		return 16
	default:
		return 2
	}
}

func roomMaxPlayers(room *GameRoom) int {
	if gs, ok := room.GameData.(dynamicJoinState); ok {
		return gs.MaxPlayers()
	}
	return defaultMaxPlayers(room.GameType)
}

func roomPlayerCount(room *GameRoom) int {
	if len(room.Participants) > 0 {
		return len(room.Participants)
	}
	count := 0
	for _, sp := range room.SavedPlayers {
		if sp.Role == "player" {
			count++
		}
	}
	return count
}

func canJoinAsPlayer(room *GameRoom) bool {
	if roomPlayerCount(room) >= roomMaxPlayers(room) {
		return false
	}
	if room.Status != "playing" {
		return true
	}
	if gs, ok := room.GameData.(dynamicJoinState); ok {
		return gs.CanJoinDuringPlay()
	}
	return false
}

func checkGameFinished(room *GameRoom) {
	if gs, ok := room.GameData.(interface{ GetStatus() string }); ok {
		if gs.GetStatus() == "finished" {
			room.Status = "finished"
		}
	}
}

func (l *Lobby) persistRoomStatus(roomID, status string) {
	if err := db.UpdateRoomStatus(roomID, status); err != nil {
		fmt.Printf("warn: failed to persist room status: %v\n", err)
	}
}

func (l *Lobby) persistGameState(room *GameRoom) {
	if room.GameData == nil {
		return
	}
	var data []byte
	var err error
	if ps, ok := room.GameData.(PersistableState); ok {
		data, err = ps.MarshalState()
	} else {
		data, err = json.Marshal(room.GameData)
	}
	if err != nil {
		fmt.Printf("warn: failed to marshal game state for room %s: %v\n", room.ID, err)
		return
	}
	if err := db.SaveGameState(room.ID, room.GameType, string(data)); err != nil {
		fmt.Printf("warn: failed to persist game state for room %s: %v\n", room.ID, err)
	}
}

func (l *Lobby) saveParticipants(room *GameRoom) {
	room.Mu.RLock()
	playerIDs := make([]int64, 0)
	playerUsernames := make(map[int64]string)
	spectatorIDs := make([]int64, 0)
	spectatorUsernames := make(map[int64]string)
	for _, sp := range room.SavedPlayers {
		if sp.Role == "player" {
			playerIDs = append(playerIDs, sp.ID)
			playerUsernames[sp.ID] = sp.Username
		} else {
			spectatorIDs = append(spectatorIDs, sp.ID)
			spectatorUsernames[sp.ID] = sp.Username
		}
	}
	room.Mu.RUnlock()

	if err := db.SaveParticipants(room.ID, playerIDs, playerUsernames, spectatorIDs, spectatorUsernames); err != nil {
		fmt.Printf("warn: failed to save participants for room %s: %v\n", room.ID, err)
	}
}

func (l *Lobby) PersistRoomAndState(room *GameRoom) {
	if room.Status == "finished" {
		l.persistRoomStatus(room.ID, "finished")
	}
	l.persistGameState(room)
	l.saveParticipants(room)
}

func (l *Lobby) CreateRoom(s *session.Session, req *protocol.CreateRoomReq) error {
	uid, _, err := auth.BindPlayer(s)
	if err != nil {
		return s.Response(&protocol.Response{Code: 401, Message: err.Error()})
	}

	// If a room creation key is configured, require it
	if l.RoomCreationKey != "" && req.Key != l.RoomCreationKey {
		return s.Response(&protocol.Response{Code: 403, Message: "invalid room creation key"})
	}

	// Auto-leave any existing room before creating a new one
	if existing := l.playerInRoom(uid); existing != "" {
		l.leaveRoomForPlayer(uid)
	}

	roomID, err := l.generateRoomCode()
	if err != nil {
		return s.Response(&protocol.Response{Code: 500, Message: "failed to generate room code"})
	}
	room := &GameRoom{
		ID:           roomID,
		GameType:     req.GameType,
		HostID:       uid,
		Status:       "waiting",
		Participants: map[int64]*session.Session{uid: s},
		Spectators:   make(map[int64]*session.Session),
		SavedPlayers: []SavedPlayer{{ID: uid, Username: s.String("username"), Role: "player"}},
		Config:       req.Config,
		LastActivity: time.Now(),
	}

	l.mu.Lock()
	l.rooms[roomID] = room
	l.playerRooms[uid] = roomID
	l.mu.Unlock()

	db.Instance.Create(&db.Room{
		ID:        roomID,
		GameType:  req.GameType,
		Status:    "waiting",
		HostID:    uid,
		Config:    req.Config,
		CreatedAt: time.Now(),
		UpdatedAt: time.Now(),
	})
	l.saveParticipants(room)

	s.Push("onRoomCreated", l.toRoomInfo(room))
	return s.Response(&protocol.CreateRoomRes{Code: 0, Message: "created", RoomID: roomID})
}

func (l *Lobby) ListRooms(s *session.Session, req *protocol.ListRoomsReq) error {
	l.mu.RLock()
	defer l.mu.RUnlock()

	var rooms []protocol.RoomInfo
	for _, r := range l.rooms {
		if req.GameType != "" && r.GameType != req.GameType {
			continue
		}
		info := l.toRoomInfo(r)
		if info.Status == "finished" {
			continue
		}
		rooms = append(rooms, info)
	}

	return s.Response(&protocol.ListRoomsRes{Rooms: rooms})
}

func (l *Lobby) JoinRoom(s *session.Session, req *protocol.JoinRoomReq) error {
	uid, _, err := auth.BindPlayer(s)
	if err != nil {
		return s.Response(&protocol.Response{Code: 401, Message: err.Error()})
	}

	normID := strings.ToUpper(req.RoomID)
	mode := strings.ToLower(req.Mode)

	// Auto-leave any existing room before joining a new one
	if err := l.maybeLeaveCurrentRoom(uid, normID); err != nil {
		return err
	}

	l.mu.Lock()
	room, ok := l.rooms[normID]
	if !ok {
		l.mu.Unlock()
		return s.Response(&protocol.Response{Code: 404, Message: "room not found"})
	}

	room.Mu.Lock()
	_, inPlayers := room.Participants[uid]
	_, inSpectators := room.Spectators[uid]

	joinAsPlayer := mode == "player" || mode == ""
	if inPlayers {
		joinAsPlayer = true
	} else if room.isKnownPlayer(uid) {
		joinAsPlayer = true
	} else if mode == "spectator" {
		joinAsPlayer = false
	} else if joinAsPlayer {
		joinAsPlayer = canJoinAsPlayer(room)
	}

	if joinAsPlayer {
		delete(room.Spectators, uid)
		room.Participants[uid] = s
		if !inPlayers {
			if gs, ok := room.GameData.(dynamicJoinState); ok {
				gs.AddParticipant(uid)
			}
		}
	} else {
		if inPlayers {
			if gs, ok := room.GameData.(dynamicJoinState); ok {
				gs.RemoveParticipant(uid)
			}
		}
		delete(room.Participants, uid)
		room.Spectators[uid] = s
	}

	alreadyInSaved := false
	for _, sp := range room.SavedPlayers {
		if sp.ID == uid {
			alreadyInSaved = true
			break
		}
	}
	if !alreadyInSaved {
		role := "spectator"
		if joinAsPlayer {
			role = "player"
		}
		room.SavedPlayers = append(room.SavedPlayers, SavedPlayer{
			ID:       uid,
			Username: s.String("username"),
			Role:     role,
		})
	} else {
		for i := range room.SavedPlayers {
			if room.SavedPlayers[i].ID == uid {
				if joinAsPlayer {
					room.SavedPlayers[i].Role = "player"
				} else {
					room.SavedPlayers[i].Role = "spectator"
				}
				break
			}
		}
	}

	wasKnownPlayer := joinAsPlayer && !inPlayers && room.isKnownPlayer(uid)

	room.LastActivity = time.Now()
	l.playerRooms[uid] = normID
	room.Mu.Unlock()
	l.mu.Unlock()

	info := l.toRoomInfo(room)
	l.broadcastToRoom(room, "onRoomUpdated", info)
	l.saveParticipants(room)

	if joinAsPlayer {
		if wasKnownPlayer {
			return s.Response(&protocol.Response{Code: 0, Message: "rejoined as player"})
		}
		return s.Response(&protocol.Response{Code: 0, Message: "joined as player"})
	}
	if inSpectators {
		return s.Response(&protocol.Response{Code: 0, Message: "already spectator"})
	}
	return s.Response(&protocol.Response{Code: 0, Message: "joined as spectator"})
}

func (l *Lobby) LeaveRoom(s *session.Session, req *protocol.LeaveRoomReq) error {
	uid, _, err := auth.BindPlayer(s)
	if err != nil {
		return s.Response(&protocol.Response{Code: 401, Message: err.Error()})
	}

	normID := strings.ToUpper(req.RoomID)
	l.mu.Lock()
	room, ok := l.rooms[normID]
	if !ok {
		l.mu.Unlock()
		return s.Response(&protocol.Response{Code: 404, Message: "room not found"})
	}
	room.Mu.Lock()
	delete(room.Participants, uid)
	delete(room.Spectators, uid)
	if gs, ok := room.GameData.(dynamicJoinState); ok {
		gs.RemoveParticipant(uid)
		checkGameFinished(room)
	}
	newSavedPlayers := make([]SavedPlayer, 0, len(room.SavedPlayers))
	for _, sp := range room.SavedPlayers {
		if sp.ID != uid {
			newSavedPlayers = append(newSavedPlayers, sp)
		}
	}
	room.SavedPlayers = newSavedPlayers
	shouldDelete := len(room.SavedPlayers) == 0

	room.LastActivity = time.Now()
	delete(l.playerRooms, uid)

	var newHostID int64
	if uid == room.HostID {
		for _, sp := range room.SavedPlayers {
			if sp.ID != uid {
				room.HostID = sp.ID
				newHostID = sp.ID
				break
			}
		}
	}
	room.Mu.Unlock()

	if shouldDelete {
		delete(l.rooms, normID)
		l.mu.Unlock()
		db.DeleteRoomAndState(normID)
	} else {
		if newHostID != 0 {
			db.Instance.Model(&db.Room{}).Where("id = ?", normID).Update("host_id", newHostID)
		}
		l.mu.Unlock()
		info := l.toRoomInfo(room)
		l.broadcastToRoom(room, "onRoomUpdated", info)
		l.PersistRoomAndState(room)
	}
	return s.Response(&protocol.Response{Code: 0, Message: "left"})
}

func (l *Lobby) StartGame(s *session.Session, req *protocol.StartGameReq) error {
	uid, _, err := auth.BindPlayer(s)
	if err != nil {
		return s.Response(&protocol.Response{Code: 401, Message: err.Error()})
	}

	normID := strings.ToUpper(req.RoomID)
	l.mu.Lock()
	room, ok := l.rooms[normID]
	if !ok {
		l.mu.Unlock()
		return s.Response(&protocol.Response{Code: 404, Message: "room not found"})
	}
	room.Mu.Lock()
	if room.HostID != uid {
		room.Mu.Unlock()
		l.mu.Unlock()
		return s.Response(&protocol.Response{Code: 403, Message: "only host can start"})
	}
	if room.Status != "waiting" && room.Status != "finished" {
		room.Mu.Unlock()
		l.mu.Unlock()
		return s.Response(&protocol.Response{Code: 400, Message: "already started"})
	}
	if len(room.Participants) < 2 {
		room.Mu.Unlock()
		l.mu.Unlock()
		return s.Response(&protocol.Response{Code: 400, Message: "need at least 2 players"})
	}

	minPlayers := 2
	switch room.GameType {
	case "roulette":
		minPlayers = 3
	}
	if len(room.Participants) < minPlayers {
		room.Mu.Unlock()
		l.mu.Unlock()
		return s.Response(&protocol.Response{Code: 400, Message: fmt.Sprintf("need at least %d players", minPlayers)})
	}

	// For roulette/mafia, convert spectators to players on restart (up to max)
	if (room.GameType == "roulette" || room.GameType == "mafia") && (room.Status == "finished" || room.Status == "waiting") {
		maxPlayers := roomMaxPlayers(room)
		currentPlayers := len(room.Participants)
		available := maxPlayers - currentPlayers
		if available > 0 && len(room.Spectators) > 0 {
			converted := 0
			for sid, sess := range room.Spectators {
				if converted >= available {
					break
				}
				delete(room.Spectators, sid)
				room.Participants[sid] = sess
				for i := range room.SavedPlayers {
					if room.SavedPlayers[i].ID == sid {
						room.SavedPlayers[i].Role = "player"
						break
					}
				}
				converted++
			}
		}
	}

	room.Status = "playing"
	room.GameData = nil
	room.LastActivity = time.Now()
	room.Mu.Unlock()
	l.mu.Unlock()

	l.persistRoomStatus(normID, "playing")

	if initer, ok := l.gameIniter[room.GameType]; ok && initer != nil {
		if err := initer(room); err != nil {
			l.mu.Lock()
			if r, ok := l.rooms[normID]; ok {
				r.Mu.Lock()
				r.Status = "waiting"
				r.Mu.Unlock()
			}
			l.mu.Unlock()
			l.persistRoomStatus(normID, "waiting")
			return s.Response(&protocol.Response{Code: 500, Message: "game init failed: " + err.Error()})
		}
	}

	l.PersistRoomAndState(room)

	info := l.toRoomInfo(room)
	l.broadcastToRoom(room, "onGameStarted", info)
	return s.Response(&protocol.Response{Code: 0, Message: "started"})
}

func (l *Lobby) SetRoomGameData(roomID string, data interface{}) {
	normID := strings.ToUpper(roomID)
	l.mu.Lock()
	defer l.mu.Unlock()
	if r, ok := l.rooms[normID]; ok {
		r.Mu.Lock()
		r.GameData = data
		r.Mu.Unlock()
	}
}

func (l *Lobby) GetRoom(roomID string) (*GameRoom, error) {
	normID := strings.ToUpper(roomID)
	l.mu.RLock()
	defer l.mu.RUnlock()
	r, ok := l.rooms[normID]
	if !ok {
		return nil, fmt.Errorf("room not found")
	}
	return r, nil
}

func (l *Lobby) GetRoomInfo(s *session.Session, req *protocol.GetRoomReq) error {
	room, err := l.GetRoom(req.RoomID)
	if err != nil {
		return s.Response(&protocol.Response{Code: 404, Message: err.Error()})
	}
	return s.Response(l.toRoomInfo(room))
}

func (l *Lobby) KickPlayer(s *session.Session, req *protocol.KickPlayerReq) error {
	uid, _, err := auth.BindPlayer(s)
	if err != nil {
		return s.Response(&protocol.Response{Code: 401, Message: err.Error()})
	}
	normID := strings.ToUpper(req.RoomID)
	l.mu.Lock()
	room, ok := l.rooms[normID]
	if !ok {
		l.mu.Unlock()
		return s.Response(&protocol.Response{Code: 404, Message: "room not found"})
	}
	room.Mu.Lock()
	if room.HostID != uid {
		room.Mu.Unlock()
		l.mu.Unlock()
		return s.Response(&protocol.Response{Code: 403, Message: "only host can kick"})
	}
	if req.PlayerID == room.HostID {
		room.Mu.Unlock()
		l.mu.Unlock()
		return s.Response(&protocol.Response{Code: 400, Message: "host cannot kick self"})
	}
	kickedSessPlayer, hadPlayer := room.Participants[req.PlayerID]
	kickedSessSpectator, hadSpectator := room.Spectators[req.PlayerID]
	if hadPlayer {
		delete(room.Participants, req.PlayerID)
		if gs, ok := room.GameData.(dynamicJoinState); ok {
			gs.RemoveParticipant(req.PlayerID)
			checkGameFinished(room)
		}
	}
	if hadSpectator {
		delete(room.Spectators, req.PlayerID)
	}
	newSavedPlayers := make([]SavedPlayer, 0, len(room.SavedPlayers))
	for _, sp := range room.SavedPlayers {
		if sp.ID != req.PlayerID {
			newSavedPlayers = append(newSavedPlayers, sp)
		}
	}
	room.SavedPlayers = newSavedPlayers
	room.Mu.Unlock()
	l.mu.Unlock()
	if !hadPlayer && !hadSpectator {
		return s.Response(&protocol.Response{Code: 404, Message: "player not in room"})
	}
	if hadPlayer && kickedSessPlayer != nil {
		kickedSessPlayer.Push("onKicked", &protocol.Response{Code: 0, Message: "kicked"})
	} else if hadSpectator && kickedSessSpectator != nil {
		kickedSessSpectator.Push("onKicked", &protocol.Response{Code: 0, Message: "kicked"})
	}
	info := l.toRoomInfo(room)
	l.broadcastToRoom(room, "onRoomUpdated", info)
	l.PersistRoomAndState(room)
	return s.Response(&protocol.Response{Code: 0, Message: "kicked"})
}

func (l *Lobby) TransferHost(s *session.Session, req *protocol.TransferHostReq) error {
	uid, _, err := auth.BindPlayer(s)
	if err != nil {
		return s.Response(&protocol.Response{Code: 401, Message: err.Error()})
	}
	normID := strings.ToUpper(req.RoomID)
	l.mu.Lock()
	room, ok := l.rooms[normID]
	if !ok {
		l.mu.Unlock()
		return s.Response(&protocol.Response{Code: 404, Message: "room not found"})
	}
	room.Mu.Lock()
	if room.HostID != uid {
		room.Mu.Unlock()
		l.mu.Unlock()
		return s.Response(&protocol.Response{Code: 403, Message: "only host can transfer host"})
	}
	_, inPlayers := room.Participants[req.PlayerID]
	_, inSpectators := room.Spectators[req.PlayerID]
	if !inPlayers && !inSpectators {
		room.Mu.Unlock()
		l.mu.Unlock()
		return s.Response(&protocol.Response{Code: 400, Message: "target must be in room"})
	}
	room.HostID = req.PlayerID
	room.Mu.Unlock()
	l.mu.Unlock()
	db.Instance.Model(&db.Room{}).Where("id = ?", normID).Update("host_id", req.PlayerID)
	l.persistRoomStatus(normID, room.Status)
	info := l.toRoomInfo(room)
	l.broadcastToRoom(room, "onRoomUpdated", info)
	return s.Response(&protocol.Response{Code: 0, Message: "host updated"})
}

func (l *Lobby) SetPresence(s *session.Session, req *protocol.SetPresenceReq) error {
	uid, _, err := auth.BindPlayer(s)
	if err != nil {
		return s.Response(&protocol.Response{Code: 401, Message: err.Error()})
	}
	normID := strings.ToUpper(req.RoomID)
	mode := strings.ToLower(req.Mode)
	if mode != "player" && mode != "spectator" {
		return s.Response(&protocol.Response{Code: 400, Message: "invalid mode"})
	}
	l.mu.Lock()
	room, ok := l.rooms[normID]
	if !ok {
		l.mu.Unlock()
		return s.Response(&protocol.Response{Code: 404, Message: "room not found"})
	}
	room.Mu.Lock()
	sess, inPlayers := room.Participants[uid]
	if !inPlayers {
		sess, _ = room.Spectators[uid]
	}
	if sess == nil {
		room.Mu.Unlock()
		l.mu.Unlock()
		return s.Response(&protocol.Response{Code: 403, Message: "not in room"})
	}
	if mode == "player" {
		if inPlayers {
			room.Mu.Unlock()
			l.mu.Unlock()
			return s.Response(&protocol.Response{Code: 0, Message: "already player"})
		}
		if !canJoinAsPlayer(room) {
			room.Mu.Unlock()
			l.mu.Unlock()
			return s.Response(&protocol.Response{Code: 400, Message: "cannot switch to player right now"})
		}
		delete(room.Spectators, uid)
		room.Participants[uid] = sess
		if gs, ok := room.GameData.(dynamicJoinState); ok {
			gs.AddParticipant(uid)
		}
		for i := range room.SavedPlayers {
			if room.SavedPlayers[i].ID == uid {
				room.SavedPlayers[i].Role = "player"
				break
			}
		}
	} else {
		if uid == room.HostID {
			room.Mu.Unlock()
			l.mu.Unlock()
			return s.Response(&protocol.Response{Code: 400, Message: "host cannot spectate"})
		}
		if !inPlayers {
			room.Mu.Unlock()
			l.mu.Unlock()
			return s.Response(&protocol.Response{Code: 0, Message: "already spectator"})
		}
		delete(room.Participants, uid)
		room.Spectators[uid] = sess
		if gs, ok := room.GameData.(dynamicJoinState); ok {
			gs.RemoveParticipant(uid)
			checkGameFinished(room)
		}
		for i := range room.SavedPlayers {
			if room.SavedPlayers[i].ID == uid {
				room.SavedPlayers[i].Role = "spectator"
				break
			}
		}
	}
	room.Mu.Unlock()
	l.mu.Unlock()
	info := l.toRoomInfo(room)
	l.broadcastToRoom(room, "onRoomUpdated", info)
	l.PersistRoomAndState(room)
	return s.Response(&protocol.Response{Code: 0, Message: "presence updated"})
}

func (l *Lobby) generateRoomCode() (string, error) {
	const alphabet = "ABCDEFGHJKLMNPQRSTUVWXYZ23456789"
	for i := 0; i < 64; i++ {
		buf := make([]byte, 6)
		for j := range buf {
			n, err := rand.Int(rand.Reader, big.NewInt(int64(len(alphabet))))
			if err != nil {
				return "", err
			}
			buf[j] = alphabet[n.Int64()]
		}
		code := string(buf)
		if _, exists := l.rooms[code]; !exists {
			return code, nil
		}
	}
	return "", fmt.Errorf("unable to generate unique room code")
}

func (l *Lobby) toRoomInfo(r *GameRoom) protocol.RoomInfo {
	r.Mu.RLock()
	defer r.Mu.RUnlock()

	playerMap := make(map[int64]protocol.PlayerInfo)
	spectatorMap := make(map[int64]protocol.PlayerInfo)

	for _, sp := range r.SavedPlayers {
		if sp.Role == "player" {
			playerMap[sp.ID] = protocol.PlayerInfo{PlayerID: sp.ID, Username: sp.Username}
		} else {
			spectatorMap[sp.ID] = protocol.PlayerInfo{PlayerID: sp.ID, Username: sp.Username}
		}
	}

	for pid, sess := range r.Participants {
		playerMap[pid] = protocol.PlayerInfo{PlayerID: pid, Username: sess.String("username")}
		delete(spectatorMap, pid)
	}
	for pid, sess := range r.Spectators {
		if _, isPlayer := playerMap[pid]; !isPlayer {
			spectatorMap[pid] = protocol.PlayerInfo{PlayerID: pid, Username: sess.String("username")}
		}
	}

	players := make([]protocol.PlayerInfo, 0, len(playerMap))
	for _, p := range playerMap {
		players = append(players, p)
	}
	spectators := make([]protocol.PlayerInfo, 0, len(spectatorMap))
	for _, p := range spectatorMap {
		spectators = append(spectators, p)
	}

	hostName := ""
	for pid, sess := range r.Participants {
		if pid == r.HostID {
			hostName = sess.String("username")
			break
		}
	}
	if hostName == "" {
		for _, sp := range r.SavedPlayers {
			if sp.ID == r.HostID {
				hostName = sp.Username
				break
			}
		}
	}
	if hostName == "" {
		for pid, sess := range r.Spectators {
			if pid == r.HostID {
				hostName = sess.String("username")
				break
			}
		}
	}

	return protocol.RoomInfo{
		RoomID:     r.ID,
		GameType:   r.GameType,
		HostID:     r.HostID,
		HostName:   hostName,
		Status:     r.Status,
		Players:    players,
		Spectators: spectators,
		Config:     r.Config,
	}
}

func (l *Lobby) broadcastToRoom(r *GameRoom, route string, v interface{}) {
	r.Mu.RLock()
	defer r.Mu.RUnlock()
	for _, sess := range r.Participants {
		sess.Push(route, v)
	}
	for _, sess := range r.Spectators {
		sess.Push(route, v)
	}
}