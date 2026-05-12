package db

import "time"

type Player struct {
	ID        int64     `gorm:"primaryKey" json:"id"`
	DeviceID  string    `gorm:"uniqueIndex" json:"device_id"`
	Username  string    `json:"username"`
	CreatedAt time.Time `json:"created_at"`
	LastSeen  time.Time `json:"last_seen"`
}

type Room struct {
	ID        string    `gorm:"primaryKey" json:"id"`
	GameType  string    `json:"game_type"`
	Status    string    `json:"status"`
	HostID    int64     `json:"host_id"`
	Config    string    `json:"config"`
	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`
}

type RoomParticipant struct {
	ID        int64     `gorm:"primaryKey" json:"id"`
	RoomID    string    `gorm:"index" json:"room_id"`
	PlayerID  int64     `json:"player_id"`
	Username  string    `json:"username"`
	Role      string    `json:"role"` // "player" or "spectator"
	CreatedAt time.Time `json:"created_at"`
}

type GameState struct {
	ID        int64     `gorm:"primaryKey" json:"id"`
	RoomID    string    `gorm:"uniqueIndex" json:"room_id"`
	GameType  string    `json:"game_type"`
	StateJSON string    `gorm:"type:text" json:"state_json"`
	UpdatedAt time.Time `json:"updated_at"`
}

func SaveGameState(roomID, gameType, stateJSON string) error {
	var gs GameState
	result := Instance.Where("room_id = ?", roomID).First(&gs)
	if result.Error == nil {
		return Instance.Model(&gs).Updates(map[string]interface{}{
			"game_type":  gameType,
			"state_json": stateJSON,
			"updated_at": time.Now(),
		}).Error
	}
	return Instance.Create(&GameState{
		RoomID:    roomID,
		GameType:  gameType,
		StateJSON:  stateJSON,
		UpdatedAt:  time.Now(),
	}).Error
}

func UpdateRoomStatus(roomID, status string) error {
	return Instance.Model(&Room{}).Where("id = ?", roomID).Updates(map[string]interface{}{
		"status":     status,
		"updated_at": time.Now(),
	}).Error
}

func UpdateRoomConfig(roomID, config string) error {
	return Instance.Model(&Room{}).Where("id = ?", roomID).Updates(map[string]interface{}{
		"config":     config,
		"updated_at": time.Now(),
	}).Error
}

func SaveParticipants(roomID string, playerIDs []int64, playerUsernames map[int64]string, spectatorIDs []int64, spectatorUsernames map[int64]string) error {
	Instance.Where("room_id = ?", roomID).Delete(&RoomParticipant{})
	var participants []RoomParticipant
	for _, pid := range playerIDs {
		uname := playerUsernames[pid]
		participants = append(participants, RoomParticipant{
			RoomID:   roomID,
			PlayerID: pid,
			Username: uname,
			Role:     "player",
		})
	}
	for _, sid := range spectatorIDs {
		uname := spectatorUsernames[sid]
		participants = append(participants, RoomParticipant{
			RoomID:   roomID,
			PlayerID: sid,
			Username: uname,
			Role:     "spectator",
		})
	}
	if len(participants) > 0 {
		return Instance.Create(&participants).Error
	}
	return nil
}

func LoadParticipants(roomID string) ([]RoomParticipant, error) {
	var participants []RoomParticipant
	err := Instance.Where("room_id = ?", roomID).Find(&participants).Error
	return participants, err
}

func DeleteRoomAndState(roomID string) error {
	Instance.Where("room_id = ?", roomID).Delete(&GameState{})
	Instance.Where("room_id = ?", roomID).Delete(&RoomParticipant{})
	return Instance.Where("id = ?", roomID).Delete(&Room{}).Error
}