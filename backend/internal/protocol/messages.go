package protocol

// Common response wrapper
type Response struct {
	Code    int    `json:"code"`
	Message string `json:"message,omitempty"`
}

// Auth messages
type LoginReq struct {
	DeviceID string `json:"device_id"`
	Username string `json:"username"`
}

type LoginRes struct {
	PlayerID int64  `json:"player_id"`
	Username string `json:"username"`
}

// Lobby messages
type CreateRoomReq struct {
	GameType string `json:"game_type"`
	Config   string `json:"config,omitempty"`
}

type RoomInfo struct {
	RoomID     string       `json:"room_id"`
	GameType   string       `json:"game_type"`
	HostID     int64        `json:"host_id"`
	HostName   string       `json:"host_name"`
	Status     string       `json:"status"`
	Players    []PlayerInfo `json:"players"`
	Spectators []PlayerInfo `json:"spectators"`
	Config     string       `json:"config,omitempty"`
}

type PlayerInfo struct {
	PlayerID int64  `json:"player_id"`
	Username string `json:"username"`
}

type JoinRoomReq struct {
	RoomID string `json:"room_id"`
	Mode   string `json:"mode,omitempty"`
}

type ListRoomsReq struct {
	GameType string `json:"game_type,omitempty"`
}

type ListRoomsRes struct {
	Rooms []RoomInfo `json:"rooms"`
}

type LeaveRoomReq struct {
	RoomID string `json:"room_id"`
}

type StartGameReq struct {
	RoomID string `json:"room_id"`
}

type GetRoomReq struct {
	RoomID string `json:"room_id"`
}

type KickPlayerReq struct {
	RoomID   string `json:"room_id"`
	PlayerID int64  `json:"player_id"`
}

type TransferHostReq struct {
	RoomID   string `json:"room_id"`
	PlayerID int64  `json:"player_id"`
}

type SetPresenceReq struct {
	RoomID string `json:"room_id"`
	Mode   string `json:"mode"`
}

type CreateRoomRes struct {
	Code    int    `json:"code"`
	Message string `json:"message"`
	RoomID  string `json:"room_id"`
}
