import { useMemo } from 'react';
import { useNavigate, useParams } from 'react-router';
import { useAuthStore } from '@/stores/authStore';
import { useGameStore } from '@/stores/gameStore';
import { useGameRoomBootstrap } from '@/hooks/useGameRoomBootstrap';
import { GamePageShell } from '@/components/GamePageShell';
import { Button } from '@/components/ui/button';
import { Card, CardContent } from '@/components/ui/card';
import { toast } from 'sonner';
import { RotateCcw, Users } from 'lucide-react';

const TTT_RULES = `
Two players take turns marking spaces in a 3×3 grid.
First player to align 3 marks (row, column, or diagonal) wins.
If all cells are filled and nobody aligned 3 marks, the game is a draw.
`;

export function GameTicTacToe() {
  const navigate = useNavigate();
  const { roomCode } = useParams<{ roomCode: string }>();
  const { player } = useAuthStore();
  const { currentRoom, tttState, leaveRoom, startGame, tttMove, tttGetState, setTTTState } = useGameStore();

  const { canStart } = useGameRoomBootstrap<ReturnType<typeof useGameStore.getState>['tttState']>({
    roomCode,
    gameType: 'tictactoe',
    lobbyPath: '/lobby/tictactoe',
    gameUpdateEvent: 'onTicTacToeUpdate',
    setGameState: setTTTState,
    getStateHandler: tttGetState,
  });

  const handleCellClick = (index: number) => {
    if (!tttState || tttState.status !== 'playing') return;
    if (tttState.turn !== player?.player_id) {
      toast.info("It's not your turn");
      return;
    }
    if (tttState.board[index]) return;
    tttMove(index);
  };

  const handleLeave = async () => {
    await leaveRoom();
    navigate('/lobby/tictactoe', { replace: true });
  };

  const isMyTurn = tttState?.turn === player?.player_id;
  const amPlayerX = tttState?.player_x === player?.player_id;
  const amPlayerO = tttState?.player_o === player?.player_id;
  const myMark = amPlayerX ? 'X' : amPlayerO ? 'O' : null;

  const statusText = useMemo(() => {
    if (!tttState) {
      if (currentRoom?.status === 'waiting') return 'Waiting for players...';
      return 'Starting...';
    }
    if (tttState.status === 'finished') {
      if (tttState.is_draw) return "It's a draw!";
      if (tttState.winner === player?.player_id) return 'You won!';
      return 'You lost!';
    }
    if (isMyTurn) return 'Your turn';
    return "Opponent's turn";
  }, [currentRoom?.status, isMyTurn, player?.player_id, tttState]);

  if (!currentRoom) return null;

  return (
    <GamePageShell
      room={currentRoom}
      rules={TTT_RULES}
      onLeave={handleLeave}
      title="Tic Tac Toe"
      statusText={statusText}
      badgeText={myMark ? `You are ${myMark}` : 'Spectator'}
      badgeVariant={isMyTurn ? 'default' : 'secondary'}
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

          {tttState && (
            <div className="flex flex-col items-center gap-4">
              <div className="grid grid-cols-3 gap-2">
                {tttState.board.map((cell, i) => (
                  <button
                    key={i}
                    onClick={() => handleCellClick(i)}
                    disabled={!isMyTurn || !!cell || tttState.status === 'finished'}
                    className={`flex h-20 w-20 items-center justify-center rounded-xl text-3xl font-bold transition-all
                      ${cell === 'X' ? 'text-emerald-500' : cell === 'O' ? 'text-rose-500' : 'text-foreground'}
                      ${!cell && isMyTurn && tttState.status === 'playing' ? 'bg-muted hover:bg-primary/10 cursor-pointer' : 'bg-muted'}
                      ${!cell && (!isMyTurn || tttState.status !== 'playing') ? 'cursor-default opacity-60' : ''}
                    `}
                  >
                    {cell}
                  </button>
                ))}
              </div>

              {tttState.status === 'finished' && (
                <Button onClick={handleLeave} variant="outline" className="gap-2">
                  <RotateCcw className="h-4 w-4" />
                  Back to Lobby
                </Button>
              )}

              {tttState.status === 'finished' && canStart && (
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