import { useMemo } from 'react';
import { useNavigate, useParams } from 'react-router';
import { useAuthStore } from '@/stores/authStore';
import { useGameStore } from '@/stores/gameStore';
import { useGameRoomBootstrap } from '@/hooks/useGameRoomBootstrap';
import { GamePageShell } from '@/components/GamePageShell';
import { Button } from '@/components/ui/button';
import { Card, CardContent } from '@/components/ui/card';
import { toast } from 'sonner';
import { RotateCcw, Hand, FileText, Scissors, Users } from 'lucide-react';

const moves = [
  { id: 'rock', label: 'Rock', icon: Hand },
  { id: 'paper', label: 'Paper', icon: FileText },
  { id: 'scissors', label: 'Scissors', icon: Scissors },
];

const RPS_RULES = `
Pick Rock, Paper, or Scissors each round.
Rock beats Scissors, Scissors beats Paper, Paper beats Rock.
Best score over the configured rounds wins the match.
`;

export function GameRPS() {
  const navigate = useNavigate();
  const { roomCode } = useParams<{ roomCode: string }>();
  const { player } = useAuthStore();
  const { currentRoom, rpsState, leaveRoom, startGame, rpsPlay, rpsGetState, setRPSState } = useGameStore();

  const { canStart } = useGameRoomBootstrap<ReturnType<typeof useGameStore.getState>['rpsState']>({
    roomCode,
    gameType: 'rps',
    lobbyPath: '/lobby/rps',
    gameUpdateEvent: 'onRPSUpdate',
    setGameState: setRPSState,
    getStateHandler: rpsGetState,
  });

  const handlePlay = (move: string) => {
    if (!rpsState || rpsState.status !== 'playing') return;
    const myId = String(player?.player_id);
    if (rpsState.moves && myId in rpsState.moves) {
      toast.info('You already played this round');
      return;
    }
    rpsPlay(move);
  };

  const handleLeave = async () => {
    await leaveRoom();
    navigate('/lobby/rps', { replace: true });
  };

  const myId = String(player?.player_id);
  const hasPlayed = rpsState?.moves && myId in rpsState.moves;
  const bothPlayed = rpsState?.moves && Object.keys(rpsState.moves).length >= 2;

  const statusText = useMemo(() => {
    if (!rpsState) {
      if (currentRoom?.status === 'waiting') return 'Waiting for players...';
      return 'Starting...';
    }
    if (rpsState.status === 'finished') {
      if (rpsState.winner === 0) return "It's a draw!";
      if (rpsState.winner === player?.player_id) return 'You won the match!';
      return 'You lost the match!';
    }
    if (!hasPlayed) return 'Choose your move';
    if (!bothPlayed) return 'Waiting for opponent...';
    return `Round ${rpsState.round} result`;
  }, [bothPlayed, currentRoom?.status, hasPlayed, player?.player_id, rpsState]);

  if (!currentRoom) return null;

  const myScore = rpsState?.scores?.[myId] ?? 0;
  const opponent = currentRoom.players?.find((p) => p.player_id !== player?.player_id);
  const opponentScore = opponent ? (rpsState?.scores?.[String(opponent.player_id)] ?? 0) : 0;

  return (
    <GamePageShell
      room={currentRoom}
      rules={RPS_RULES}
      onLeave={handleLeave}
      title="Rock Paper Scissors"
      statusText={statusText}
      badgeText="Best of 3"
      maxWidthClassName="max-w-lg"
    >
      <Card className="mb-4">
        <CardContent className="p-4">
          {currentRoom.status === 'waiting' && (
            <div className="flex flex-col items-center gap-3 py-6">
              <div className="flex items-center gap-2 text-sm text-muted-foreground">
                <Users className="h-4 w-4" />
                {currentRoom.players?.length || 0}/2 players
              </div>
              {canStart && (
                <Button onClick={startGame} className="gap-2">
                  <RotateCcw className="h-4 w-4" />
                  Start Game
                </Button>
              )}
              <div className="text-xs text-muted-foreground">
                Room: <span className="font-mono">{currentRoom.room_id}</span>
              </div>
            </div>
          )}

          {rpsState && (
            <div className="flex flex-col items-center gap-6">
              <div className="flex w-full items-center justify-between rounded-lg bg-muted p-3">
                <div className="text-center">
                  <div className="text-sm font-medium">{player?.username}</div>
                  <div className="text-2xl font-bold">{myScore}</div>
                </div>
                <div className="text-xs text-muted-foreground">
                  Round {rpsState.round}/{rpsState.max_rounds}
                </div>
                <div className="text-center">
                  <div className="text-sm font-medium">{opponent?.username || 'Opponent'}</div>
                  <div className="text-2xl font-bold">{opponentScore}</div>
                </div>
              </div>

              {rpsState.status === 'playing' && (
                <div className="grid grid-cols-3 gap-3">
                  {moves.map((m) => {
                    const Icon = m.icon;
                    const disabled = hasPlayed && !rpsState.revealed;
                    return (
                      <Button
                        key={m.id}
                        variant={hasPlayed && rpsState.moves?.[myId] === m.id ? 'default' : 'outline'}
                        className="flex h-24 flex-col items-center gap-2"
                        onClick={() => handlePlay(m.id)}
                        disabled={disabled}
                      >
                        <Icon className="h-6 w-6" />
                        <span className="text-xs">{m.label}</span>
                      </Button>
                    );
                  })}
                </div>
              )}

              {rpsState.revealed && bothPlayed && (
                <div className="flex w-full items-center justify-around rounded-lg border p-4">
                  <div className="text-center">
                    <div className="mb-1 text-xs text-muted-foreground">You</div>
                    <div className="text-lg font-bold capitalize">{rpsState.moves?.[myId]}</div>
                  </div>
                  <div className="text-xl font-bold text-muted-foreground">VS</div>
                  <div className="text-center">
                    <div className="mb-1 text-xs text-muted-foreground">Opponent</div>
                    <div className="text-lg font-bold capitalize">
                      {opponent
                        ? rpsState.moves?.[String(opponent.player_id)] || '?'
                        : '?'}
                    </div>
                  </div>
                </div>
              )}

              {rpsState.status === 'finished' && canStart && (
                <Button onClick={startGame} className="gap-2">
                  <RotateCcw className="h-4 w-4" />
                  Play Again
                </Button>
              )}
            </div>
          )}
        </CardContent>
      </Card>
    </GamePageShell>
  );
}
