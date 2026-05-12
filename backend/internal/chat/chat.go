package chat

import (
	"fmt"
	"strings"
	"sync"
	"time"

	"github.com/cinnabar-games/backend/internal/auth"
	"github.com/cinnabar-games/backend/internal/lobby"
	"github.com/cinnabar-games/backend/internal/protocol"
	"github.com/lonng/nano/component"
	"github.com/lonng/nano/session"
)

const MaxMessagesPerRoom = 100

type ChatTabDef struct {
	Name       string `json:"name"`
	Label      string `json:"label"`
	Visibility string `json:"visibility"`
	SendableBy string `json:"sendable_by"`
	CanSend    bool   `json:"can_send"`
}

type ChatMessage struct {
	PlayerID  int64  `json:"player_id"`
	Username  string `json:"username"`
	Content   string `json:"content"`
	Tab       string `json:"tab"`
	Timestamp int64  `json:"timestamp"`
}

type ChatRoleProvider interface {
	GetPlayerChatRole(playerID int64) string
}

type roomChatState struct {
	mu       sync.RWMutex
	tabs     []ChatTabDef
	messages []ChatMessage
}

type Chat struct {
	component.Base
	lobby      *lobby.Lobby
	mu         sync.RWMutex
	chatStates map[string]*roomChatState
}

type SendMessageReq struct {
	RoomID  string `json:"room_id"`
	Tab     string `json:"tab"`
	Content string `json:"content"`
}

type GetMessagesReq struct {
	RoomID string `json:"room_id"`
}

type GetMessagesRes struct {
	Tabs     []ChatTabDef  `json:"tabs"`
	Messages []ChatMessage `json:"messages"`
}

func NewChat(lob *lobby.Lobby) *Chat {
	return &Chat{
		lobby:      lob,
		chatStates: make(map[string]*roomChatState),
	}
}

func (c *Chat) SendMessage(s *session.Session, req *SendMessageReq) error {
	uid, username, err := auth.BindPlayer(s)
	if err != nil {
		return s.Response(&protocol.Response{Code: 401, Message: err.Error()})
	}

	room, err := c.lobby.GetRoom(req.RoomID)
	if err != nil {
		return s.Response(&protocol.Response{Code: 404, Message: "room not found"})
	}

	content := strings.TrimSpace(req.Content)
	if content == "" {
		return s.Response(&protocol.Response{Code: 400, Message: "empty message"})
	}
	if len(content) > 500 {
		return s.Response(&protocol.Response{Code: 400, Message: "message too long"})
	}

	cs := c.getOrCreateChatState(req.RoomID)

	cs.mu.RLock()
	var tab *ChatTabDef
	for i := range cs.tabs {
		if cs.tabs[i].Name == req.Tab {
			tab = &cs.tabs[i]
			break
		}
	}
	if tab == nil {
		cs.mu.RUnlock()
		return s.Response(&protocol.Response{Code: 400, Message: fmt.Sprintf("tab %q not found", req.Tab)})
	}
	if !c.canAccessTab(room, uid, *tab, tab.SendableBy) {
		cs.mu.RUnlock()
		return s.Response(&protocol.Response{Code: 403, Message: "you cannot send to this tab"})
	}
	cs.mu.RUnlock()

	msg := ChatMessage{
		PlayerID:  uid,
		Username:  username,
		Content:   content,
		Tab:       req.Tab,
		Timestamp: time.Now().UnixMilli(),
	}

	cs.mu.Lock()
	cs.messages = append(cs.messages, msg)
	if len(cs.messages) > MaxMessagesPerRoom {
		cs.messages = cs.messages[len(cs.messages)-MaxMessagesPerRoom:]
	}
	cs.mu.Unlock()

	room.Mu.RLock()
	defer room.Mu.RUnlock()
	for pid, sess := range room.Participants {
		if c.canAccessTab(room, pid, *tab, tab.Visibility) {
			sess.Push("onChatMessage", msg)
		}
	}
	for pid, sess := range room.Spectators {
		if c.canAccessTab(room, pid, *tab, tab.Visibility) {
			sess.Push("onChatMessage", msg)
		}
	}

	return s.Response(&protocol.Response{Code: 0})
}

func (c *Chat) GetMessages(s *session.Session, req *GetMessagesReq) error {
	uid, _, err := auth.BindPlayer(s)
	if err != nil {
		return s.Response(&protocol.Response{Code: 401, Message: err.Error()})
	}

	room, err := c.lobby.GetRoom(req.RoomID)
	if err != nil {
		return s.Response(&protocol.Response{Code: 404, Message: "room not found"})
	}

	cs := c.getOrCreateChatState(req.RoomID)

	cs.mu.RLock()
	defer cs.mu.RUnlock()

	visibleTabs := make([]ChatTabDef, 0)
	for _, tab := range cs.tabs {
		if c.canAccessTab(room, uid, tab, tab.Visibility) {
			tab.CanSend = c.canAccessTab(room, uid, tab, tab.SendableBy)
			visibleTabs = append(visibleTabs, tab)
		}
	}

	visibleMap := make(map[string]bool, len(visibleTabs))
	for _, t := range visibleTabs {
		visibleMap[t.Name] = true
	}

	visibleMessages := make([]ChatMessage, 0, len(cs.messages))
	for _, msg := range cs.messages {
		if visibleMap[msg.Tab] {
			visibleMessages = append(visibleMessages, msg)
		}
	}
	if visibleMessages == nil {
		visibleMessages = []ChatMessage{}
	}

	return s.Response(&GetMessagesRes{
		Tabs:     visibleTabs,
		Messages: visibleMessages,
	})
}

func (c *Chat) canAccessTab(room *lobby.GameRoom, uid int64, tab ChatTabDef, requiredRole string) bool {
	switch requiredRole {
	case "all":
		return true
	case "players":
		_, ok := room.Participants[uid]
		return ok
	case "spectators":
		_, ok := room.Spectators[uid]
		return ok
	default:
		if provider, ok := room.GameData.(ChatRoleProvider); ok {
			return provider.GetPlayerChatRole(uid) == requiredRole
		}
		return false
	}
}

func (c *Chat) getOrCreateChatState(roomID string) *roomChatState {
	normID := strings.ToUpper(roomID)
	c.mu.Lock()
	defer c.mu.Unlock()
	if cs, ok := c.chatStates[normID]; ok {
		return cs
	}
	cs := &roomChatState{
		tabs: []ChatTabDef{
			{Name: "general", Label: "General", Visibility: "all", SendableBy: "all"},
		},
		messages: make([]ChatMessage, 0, MaxMessagesPerRoom),
	}
	c.chatStates[normID] = cs
	return cs
}

func (c *Chat) RegisterRoomTabs(roomID string, tabs []ChatTabDef) {
	cs := c.getOrCreateChatState(roomID)
	cs.mu.Lock()
	defer cs.mu.Unlock()
	if tabs == nil {
		cs.tabs = []ChatTabDef{
			{Name: "general", Label: "General", Visibility: "all", SendableBy: "all"},
		}
	} else {
		cs.tabs = tabs
	}
}
