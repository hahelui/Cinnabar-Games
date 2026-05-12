# Cinnabar Games - Agent Guide

## Project Overview
A self-hosted mini-game website for friends (Discord group). Players join via browser, use simple guest auth, create/join game rooms, and play together in real-time.

## Architecture
- **Frontend**: React + Vite + TypeScript + Tailwind CSS v4 + shadcn/ui (in `frontend/`)
- **Backend**: Go + Nano game server framework + SQLite (in `backend/`)
- **Communication**: WebSocket with JSON serialization
- **Auth**: Guest-only (device_id + username), no passwords/OAuth

## Backend Structure
```
backend/
  cmd/server/main.go          # Entry point: wires components, starts cleanup goroutine
  configs/config.yaml         # Port, DB path, game settings
  internal/
    auth/guest.go             # GuestAuth component: login, session binding
    db/db.go                  # SQLite/GORM init + auto-migration (Player, Room, GameState, RoomParticipant)
    db/models.go              # DB models + persistence helpers (SaveGameState, SaveParticipants, etc.)
    lobby/lobby.go            # Lobby component: rooms, persistence, rejoin logic, cleanup goroutine
    protocol/messages.go      # Shared request/response structs (RoomInfo, PlayerInfo, etc.)
    games/
      tictactoe/tictactoe.go  # TicTacToe game component
      rps/rps.go              # Rock Paper Scissors game component
      roulette/roulette.go    # Roulette elimination wheel component
```

## Frontend Structure
```
frontend/src/
  pages/
    Dashboard.tsx         # Game selection cards
    Lobby.tsx             # Room list with Rejoin/Spectate buttons for active games
    GameTicTacToe.tsx      # TTT game page
    GameRPS.tsx            # RPS game page
    GameRoulette.tsx       # Roulette game page (phase-driven UI, canvas wheel)
  stores/
    authStore.ts           # Zustand auth state (player_id, username, isLoggedIn)
    gameStore.ts           # Zustand game state (rooms, currentRoom, TTT/RPS/Roulette state, actions)
  hooks/
    useGameRoomBootstrap.ts # WS connect, join room, auto-fetch state on reconnect
  components/
    RouletteWheel.tsx       # Canvas-based wheel with spin physics, section destruction
    GamePageShell.tsx       # Shared game page layout (title, status, badge, rules)
    GameHeader.tsx          # Top bar with room info, rules dialog, leave button
    GameLayout.tsx          # Full-height layout (h-svh overflow-hidden)
    Layout.tsx              # Main app layout
    AuthInitializer.tsx     # Auto-login on mount
    UsernameModal.tsx       # Username prompt
    theme-provider.tsx      # Theme context
```

## How to Add a New Game
1. Create folder `backend/internal/games/<yourgame>/`
2. Create component struct with `component.Base` and `lobby *lobby.Lobby`
3. Implement handlers (e.g., `MakeMove`) using `auth.BindPlayer(s)` to get player ID
4. Implement `InitGame(room *lobby.GameRoom) error` to set initial `room.GameData`
5. Implement `MarshalState() ([]byte, error)` for persistence (use Alias pattern for unexported fields)
6. Implement `BlankState() interface{}` for DB restore
7. Implement `RestoreGame(room *lobby.GameRoom)` if the game has timers
8. Register in `main.go`:
   ```go
   myGame := yourgame.NewYourGame(lob)
   lob.RegisterGame("yourgame", myGame.InitGame)
   lob.RegisterRestorer("yourgame", myGame.RestoreGame)  // if game has timers
   lob.RegisterBlankState("yourgame", func() interface{} { return yourgame.BlankState() })
   components.Register(myGame)
   ```
9. Add frontend client routes matching handler names (e.g., `YourGame.MakeMove`)

## Nano Routing Convention
- Client sends to `Component.Handler`
- Server pushes to client with `session.Push("onEventName", payload)`
- Example: client sends `TicTacToe.MakeMove`, server broadcasts `onTicTacToeUpdate`

## Build & Run
You dont need to build anything leave that to user, he will test and give you result is anything needed to be done

## Persistence System
- **Game state**: Saved to `game_states` table on every state-changing broadcast via `lobby.PersistRoomAndState()`
- **Room status**: Persisted on create/start/finish via `db.UpdateRoomStatus()`
- **Participants**: Saved to `room_participants` table (player_id, username, role) on every join/leave/kick/presence change
- **On server restart**: `LoadFromDB()` restores rooms with status "playing"/"finished", unmarshals saved state, restores participant list into `SavedPlayers`
- **`SavedPlayers`**: In-memory slice on `GameRoom` that tracks who belongs to a room and their role (player/spectator). Source of truth when sessions are disconnected (e.g. after server restart, browser refresh during game)
- **Key interfaces**: `PersistableState` (custom JSON marshal), `dynamicJoinState` (CanJoinDuringPlay, AddParticipant, RemoveParticipant)

## Rejoin & Role Restoration
- When a player rejoins a room, `JoinRoom` checks `SavedPlayers` to determine their previous role
- Known players always rejoin as players (bypasses `canJoinAsPlayer` check)
- Unknown players join as spectators (or players if room is waiting/seats available)
- `toRoomInfo` merges `SavedPlayers` with live session data so disconnected players still appear in room info
- `saveParticipants` uses `SavedPlayers` (not live sessions) as source of truth to avoid wiping data on disconnect

## Roulette "Play Again"
- When roulette host clicks "Play Again", `StartGame` converts all spectators to players (up to max 20)
- Remaining spectators stay as spectators; roles are updated in both sessions and `SavedPlayers`

## Lobby Buttons
- **Waiting rooms**: Show "Join" button for all users
- **Playing/finished rooms**: Show "Rejoin" for players in the room's `players` list, "Spectate" for everyone else
- Frontend checks `currentRoom.players` array against `authStore.player.player_id`

## Room Cleanup
- `StartCleanup()` runs a goroutine every 30 seconds
- Deletes rooms with no active sessions where idle time exceeds TTL:
  - `waiting` rooms with no saved players: 10 min
  - `waiting`/`playing` rooms with saved players: 2 hours
  - `finished` rooms: 5 min
- `GameRoom.LastActivity` is updated on create, join, leave, start
- Active sessions reset `LastActivity` on every cleanup check

## Important Notes
- **Session binding**: Always call `s.Bind(playerID)` after login so `s.UID()` works
- **Room mutex**: Game components must lock `room.Mu` before accessing `room.GameData`
- **SQLite**: Single file DB at `backend/data/cinnabar.db` — zero setup
- **CORS**: Backend allows all origins for local dev (`nano.SetCheckOriginFunc`)
- **Game pages use `h-svh overflow-hidden`**: No scrolling, content fits viewport
- **`useGameRoomBootstrap`**: Takes `getStateHandler` param to auto-fetch game state on reconnect; also calls `getRoomInfo` when received `status: "finished"` to enable "Play Again"
- **Roulette wheel**: Frontend calculates landing angle from `selectedIndex` with 4-6 random full rotations; uses refs for stable animation loop
- **Roulette timer restoration**: `RestoreGame` reschedules any active timers from `phase_ends_at` and `decision_deadline` fields