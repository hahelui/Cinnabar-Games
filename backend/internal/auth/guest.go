package auth

import (
	"errors"
	"fmt"
	"time"
	"unicode/utf8"

	"github.com/cinnabar-games/backend/internal/db"
	"github.com/cinnabar-games/backend/internal/protocol"
	"github.com/google/uuid"
	"github.com/lonng/nano/component"
	"github.com/lonng/nano/session"
	"gorm.io/gorm"
)

type GuestAuth struct {
	component.Base
	OnSessionBind func(uid int64, sess *session.Session)
}

func NewGuestAuth() *GuestAuth {
	return &GuestAuth{}
}

// GuestLogin creates or retrieves a player by device_id
func (a *GuestAuth) GuestLogin(s *session.Session, req *protocol.LoginReq) error {
	if req.DeviceID == "" {
		req.DeviceID = uuid.New().String()
	}
	if req.Username == "" {
		req.Username = "Guest"
	}
	// Truncate to 14 characters
	if utf8.RuneCountInString(req.Username) > 14 {
		req.Username = string([]rune(req.Username)[:14])
	}

	var player db.Player
	result := db.Instance.Where("device_id = ?", req.DeviceID).First(&player)
	if errors.Is(result.Error, gorm.ErrRecordNotFound) {
		// Create new player
		player = db.Player{
			DeviceID:  req.DeviceID,
			Username:  req.Username,
			CreatedAt: time.Now(),
			LastSeen:  time.Now(),
		}
		if err := db.Instance.Create(&player).Error; err != nil {
			return s.Response(&protocol.Response{Code: 500, Message: "failed to create player: " + err.Error()})
		}
	} else if result.Error != nil {
		return s.Response(&protocol.Response{Code: 500, Message: "database error: " + result.Error.Error()})
	} else {
		// Update username if changed and last seen
		if req.Username != "" && req.Username != player.Username {
			player.Username = req.Username
		}
		player.LastSeen = time.Now()
		db.Instance.Save(&player)
	}

	// Bind session to player ID so Nano knows who this is
	s.Bind(player.ID)
	s.Set("username", player.Username)

	if a.OnSessionBind != nil {
		a.OnSessionBind(player.ID, s)
	}

	return s.Response(&protocol.LoginRes{
		PlayerID: player.ID,
		Username: player.Username,
	})
}

// GetPlayerInfo returns current session info
func (a *GuestAuth) GetPlayerInfo(s *session.Session, msg []byte) error {
	uid := s.UID()
	if uid == 0 {
		return s.Response(&protocol.Response{Code: 401, Message: "not logged in"})
	}

	var player db.Player
	if err := db.Instance.First(&player, uid).Error; err != nil {
		return s.Response(&protocol.Response{Code: 404, Message: "player not found"})
	}

	return s.Response(&protocol.LoginRes{
		PlayerID: player.ID,
		Username: player.Username,
	})
}

// BindPlayer is an internal helper to ensure a session is authenticated
func BindPlayer(s *session.Session) (int64, string, error) {
	uid := s.UID()
	if uid == 0 {
		return 0, "", fmt.Errorf("not authenticated")
	}
	nameStr := s.String("username")
	return uid, nameStr, nil
}
