package main

import (
	"log"
	"net/http"
	"os"

	"github.com/cinnabar-games/backend/internal/auth"
	"github.com/cinnabar-games/backend/internal/chat"
	"github.com/cinnabar-games/backend/internal/db"
"github.com/cinnabar-games/backend/internal/games/almuamara"
	"github.com/cinnabar-games/backend/internal/games/mafia"
	"github.com/cinnabar-games/backend/internal/games/rps"
	"github.com/cinnabar-games/backend/internal/games/roulette"
	"github.com/cinnabar-games/backend/internal/games/tictactoe"
	"github.com/cinnabar-games/backend/internal/lobby"
	"github.com/lonng/nano"
	"github.com/lonng/nano/component"
	"github.com/lonng/nano/serialize/json"
	"gopkg.in/yaml.v3"
)

type Config struct {
	Port            string `yaml:"port"`
	WebSocketPath   string `yaml:"websocket_path"`
	DBPath          string `yaml:"db_path"`
	RoomCreationKey string `yaml:"room_creation_key"`
}

func main() {
	cfg := loadConfig()

	if err := db.Init(cfg.DBPath); err != nil {
		log.Fatalf("db init failed: %v", err)
	}

	lob := lobby.NewLobby()
	lob.RoomCreationKey = cfg.RoomCreationKey
	chatComp := chat.NewChat(lob)
	authComp := auth.NewGuestAuth()
	authComp.OnSessionBind = lob.RegisterActiveSession
	ttt := tictactoe.NewTicTacToe(lob)
	rpsComp := rps.NewRPS(lob)
	rouletteComp := roulette.NewRoulette(lob)
	muamaraComp := almuamara.NewAlMuamara(lob)
	mafiaComp := mafia.NewMafia(lob, chatComp)

	lob.RegisterGame("tictactoe", ttt.InitGame)
	lob.RegisterGame("rps", rpsComp.InitGame)
	lob.RegisterGame("roulette", rouletteComp.InitGame)
lob.RegisterGame("almuamara", muamaraComp.InitGame)
	lob.RegisterGame("mafia", mafiaComp.InitGame)
	lob.RegisterRestorer("roulette", rouletteComp.RestoreGame)
	lob.RegisterRestorer("almuamara", muamaraComp.RestoreGame)
	lob.RegisterRestorer("mafia", mafiaComp.RestoreGame)
	lob.RegisterBlankState("tictactoe", func() interface{} { return tictactoe.BlankState() })
	lob.RegisterBlankState("rps", func() interface{} { return rps.BlankState() })
	lob.RegisterBlankState("roulette", func() interface{} { return roulette.BlankState() })
	lob.RegisterBlankState("almuamara", func() interface{} { return almuamara.BlankState() })
	lob.RegisterBlankState("mafia", func() interface{} { return mafia.BlankState() })

	lob.LoadFromDB()
	lob.StartCleanup()

	components := &component.Components{}
	components.Register(authComp)
	components.Register(chatComp)
	components.Register(lob)
	components.Register(ttt)
	components.Register(rpsComp)
	components.Register(rouletteComp)
	components.Register(muamaraComp)
	components.Register(mafiaComp)

	log.SetFlags(log.LstdFlags | log.Llongfile)

	nano.Listen(":"+cfg.Port,
		nano.WithIsWebsocket(true),
		nano.WithWSPath(cfg.WebSocketPath),
		nano.WithCheckOriginFunc(func(_ *http.Request) bool { return true }),
		nano.WithDebugMode(),
		nano.WithSerializer(json.NewSerializer()),
		nano.WithComponents(components),
	)
}

func loadConfig() Config {
	data, err := os.ReadFile("configs/config.yaml")
	if err != nil {
		log.Printf("config file not found, using defaults: %v", err)
		return Config{Port: "3250", WebSocketPath: "/ws", DBPath: "./data/cinnabar.db"}
	}
	var cfg Config
	if err := yaml.Unmarshal(data, &cfg); err != nil {
		log.Fatalf("invalid config: %v", err)
	}
	if cfg.Port == "" {
		cfg.Port = "3250"
	}
	if cfg.WebSocketPath == "" {
		cfg.WebSocketPath = "/ws"
	}
	if cfg.DBPath == "" {
		cfg.DBPath = "./data/cinnabar.db"
	}
	return cfg
}
