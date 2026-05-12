import { useCallback, useEffect, useMemo, useRef, useState } from 'react';
import { useNavigate, useParams } from 'react-router';
import { useAuthStore } from '@/stores/authStore';
import { useGameStore } from '@/stores/gameStore';
import { useGameRoomBootstrap } from '@/hooks/useGameRoomBootstrap';
import { GamePageShell } from '@/components/GamePageShell';
import { RouletteWheel } from '@/components/RouletteWheel';
import { Button } from '@/components/ui/button';
import { Badge } from '@/components/ui/badge';
import {
  Dialog,
  DialogContent,
  DialogHeader,
  DialogTitle,
  DialogDescription,
} from '@/components/ui/dialog';
import { RotateCcw, Users, Clock, Target, LogOut, UserMinus, History } from 'lucide-react';
import { toast } from 'sonner';

const ROULETTE_RULES = `
Elimination Roulette: The wheel decides, you choose!

3-20 players take turns:
1. The wheel spins and selects one player.
2. The chosen player kicks someone off the wheel or withdraws.
3. If they don't decide within 30s, the wheel kicks them!

Last player standing wins.
`;

export function GameRoulette() {
  const navigate = useNavigate();
  const { roomCode } = useParams<{ roomCode: string }>();
  const { player } = useAuthStore();
  const {
    currentRoom,
    rouletteState,
    leaveRoom,
    startGame,
    rouletteChoose,
    rouletteGetState,
    setRouletteState,
  } = useGameStore();

  const { canStart } = useGameRoomBootstrap<ReturnType<typeof useGameStore.getState>['rouletteState']>({
    roomCode,
    gameType: 'roulette',
    lobbyPath: '/lobby/roulette',
    gameUpdateEvent: 'onRouletteUpdate',
    setGameState: setRouletteState,
    getStateHandler: rouletteGetState,
  });

  const [isSpinning, setIsSpinning] = useState(false);
  const [spinningTriggered, setSpinningTriggered] = useState(false);
  const [countdown, setCountdown] = useState<number | null>(null);
  const [decisionTimeLeft, setDecisionTimeLeft] = useState<number | null>(null);
  const [statusMessage, setStatusMessage] = useState('');
  const [showHistory, setShowHistory] = useState(false);
  const prevPhaseRef = useRef<string>('');
  const countdownRef = useRef<ReturnType<typeof setInterval> | null>(null);
  const decisionRef = useRef<ReturnType<typeof setInterval> | null>(null);

  useEffect(() => {
    if (!rouletteState) return;
    const phase = rouletteState.phase;

    if (phase === 'countdown' && prevPhaseRef.current !== 'countdown') {
      setIsSpinning(false);
      setSpinningTriggered(false);
    }
    if (phase === 'spinning' && !spinningTriggered) {
      setIsSpinning(true);
      setSpinningTriggered(true);
    }
    if (phase === 'deciding' && prevPhaseRef.current !== 'deciding') {
      setIsSpinning(false);
      setSpinningTriggered(false);
    }
    if (phase === 'result' && prevPhaseRef.current !== 'result') {
      setIsSpinning(false);
      setSpinningTriggered(false);
    }
    if (phase === 'finished') {
      setIsSpinning(false);
      setSpinningTriggered(false);
    }

    prevPhaseRef.current = phase;
  }, [rouletteState, spinningTriggered]);

  useEffect(() => {
    if (!rouletteState) return;

    if (rouletteState.phase === 'countdown') {
      if (countdownRef.current) clearInterval(countdownRef.current);
      const update = () => {
        const remaining = rouletteState!.phase_ends_at - Date.now();
        if (remaining <= 0) {
          setCountdown(0);
          if (countdownRef.current) clearInterval(countdownRef.current);
          return;
        }
        setCountdown(Math.ceil(remaining / 1000));
      };
      update();
      countdownRef.current = setInterval(update, 100);
    } else {
      setCountdown(null);
      if (countdownRef.current) { clearInterval(countdownRef.current); countdownRef.current = null; }
    }
    return () => { if (countdownRef.current) { clearInterval(countdownRef.current); countdownRef.current = null; } };
  }, [rouletteState?.phase, rouletteState?.phase_ends_at]);

  useEffect(() => {
    if (!rouletteState) return;

    if (rouletteState.phase === 'deciding' && rouletteState.decision_deadline) {
      if (decisionRef.current) clearInterval(decisionRef.current);
      const update = () => {
        const remaining = rouletteState!.decision_deadline - Date.now();
        if (remaining <= 0) { setDecisionTimeLeft(0); if (decisionRef.current) clearInterval(decisionRef.current); return; }
        setDecisionTimeLeft(Math.ceil(remaining / 1000));
      };
      update();
      decisionRef.current = setInterval(update, 200);
    } else {
      setDecisionTimeLeft(null);
      if (decisionRef.current) { clearInterval(decisionRef.current); decisionRef.current = null; }
    }
    return () => { if (decisionRef.current) { clearInterval(decisionRef.current); decisionRef.current = null; } };
  }, [rouletteState?.phase, rouletteState?.decision_deadline]);

  useEffect(() => {
    if (!rouletteState) { setStatusMessage(''); return; }
    const phase = rouletteState.phase;
    const isMe = rouletteState.selected_id === player?.player_id;

    switch (phase) {
      case 'countdown':
        setStatusMessage(countdown !== null && countdown > 0 ? `Spinning in ${countdown}...` : 'Get ready...');
        break;
      case 'spinning':
        setStatusMessage('The wheel is spinning...');
        break;
      case 'deciding':
        if (isMe) setStatusMessage("You're chosen! Make your move");
        else setStatusMessage(rouletteState.last_event || `Waiting for ${getSelectedName()}...`);
        break;
      case 'result':
        setStatusMessage(rouletteState.last_event || 'Round result');
        break;
      case 'finished':
        setStatusMessage(rouletteState.winner_name ? `${rouletteState.winner_name} wins!` : 'Game over!');
        break;
      default:
        setStatusMessage('');
    }
  }, [rouletteState, countdown, player?.player_id]);

  const getSelectedName = useCallback(() => {
    if (!rouletteState || !rouletteState.selected_id) return '...';
    const selected = rouletteState.players.find((p) => p.id === rouletteState.selected_id);
    return selected?.username || '...';
  }, [rouletteState]);

  const handleChoose = useCallback(async (action: string, targetId?: number) => {
    try { await rouletteChoose(action, targetId); } catch { toast.error('Action failed'); }
  }, [rouletteChoose]);

  const handleLeave = useCallback(async () => {
    await leaveRoom();
    navigate('/lobby/roulette', { replace: true });
  }, [leaveRoom, navigate]);

  const handleSpinComplete = useCallback(() => { setIsSpinning(false); }, []);

  const kickablePlayers = useMemo(() => {
    if (!rouletteState || !player) return [];
    return rouletteState.players.filter((p) => p.id !== player.player_id && p.id !== rouletteState.selected_id);
  }, [rouletteState, player]);

  const isSelectedPlayer = rouletteState?.selected_id === player?.player_id;
  const isDeciding = rouletteState?.phase === 'deciding';
  const showChoiceDialog = isDeciding && isSelectedPlayer;

  if (!currentRoom) return null;

  return (
    <GamePageShell
      room={currentRoom}
      rules={ROULETTE_RULES}
      onLeave={handleLeave}
      title="Roulette"
      statusText={statusMessage}
      badgeText={rouletteState ? `Round ${rouletteState.round}` : currentRoom.status === 'waiting' ? `${currentRoom.players?.length || 0}/20` : undefined}
      maxWidthClassName="max-w-2xl"
    >
      {currentRoom.status === 'waiting' && (
        <div className="flex flex-col items-center gap-3 py-8">
          <div className="flex items-center gap-2 text-sm text-muted-foreground">
            <Users className="h-4 w-4" />
            {currentRoom.players?.length || 0}/3-20 players
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

      {rouletteState && (
        <div className="flex min-h-0 flex-1 flex-col items-center gap-2">
          <div className="flex w-full items-center gap-2 rounded-lg border bg-card px-3 py-2 text-sm">
            <div className="flex items-center gap-1.5">
              <Target className="h-4 w-4 text-primary" />
              <span className="font-medium">{rouletteState.players.length} left</span>
            </div>
            {decisionTimeLeft !== null && decisionTimeLeft > 0 && (
              <Badge variant={decisionTimeLeft <= 10 ? 'destructive' : 'secondary'} className="gap-1 shrink-0">
                <Clock className="h-3 w-3" />
                {decisionTimeLeft}s
              </Badge>
            )}
            {rouletteState.eliminated.length > 0 && (
              <Button variant="ghost" size="icon" className="h-7 w-7 shrink-0" onClick={() => setShowHistory(true)}>
                <History className="h-4 w-4" />
              </Button>
            )}
          </div>

          <div className="relative flex min-h-0 w-full max-w-md flex-1 items-center justify-center">
            <RouletteWheel
              players={rouletteState.players}
              selectedIndex={rouletteState.selected_index}
              isSpinning={isSpinning}
              spinDurationMs={rouletteState.spin_duration_ms || 5500}
              eliminatedIds={rouletteState.eliminated.map((e) => e.player_id)}
              onSpinComplete={handleSpinComplete}
            />
            {rouletteState.phase === 'countdown' && countdown !== null && countdown > 0 && (
              <div className="pointer-events-none absolute inset-0 flex items-center justify-center">
                <span className="animate-pulse text-7xl font-black text-white drop-shadow-[0_2px_12px_rgba(0,0,0,0.9)]">
                  {countdown}
                </span>
              </div>
            )}
          </div>

          {rouletteState.phase === 'finished' && (
            <div className="flex gap-2">
              {canStart && (
                <Button onClick={startGame} size="sm" className="gap-1.5">
                  <RotateCcw className="h-4 w-4" />
                  Play Again
                </Button>
              )}
              <Button variant="outline" onClick={handleLeave} size="sm" className="gap-1.5">
                <LogOut className="h-4 w-4" />
                Back to Lobby
              </Button>
            </div>
          )}

          <Dialog open={showChoiceDialog} onOpenChange={() => {}}>
            <DialogContent className="sm:max-w-sm" onPointerDownOutside={(e) => e.preventDefault()} onEscapeKeyDown={(e) => e.preventDefault()}>
              <DialogHeader>
                <DialogTitle>You were chosen!</DialogTitle>
                <DialogDescription>Kick someone off the wheel or withdraw.</DialogDescription>
              </DialogHeader>
              <div className="space-y-3">
                {kickablePlayers.length > 0 && (
                  <div className="space-y-1.5">
                    <div className="text-xs font-medium text-muted-foreground">Kick a player:</div>
                    <div className="flex max-h-48 flex-col gap-1.5 overflow-y-auto">
                      {kickablePlayers.map((p) => (
                        <Button key={p.id} variant="destructive" size="sm" className="justify-start gap-2" onClick={() => handleChoose('kick', p.id)}>
                          <UserMinus className="h-3.5 w-3.5" />
                          Kick {p.username}
                        </Button>
                      ))}
                    </div>
                  </div>
                )}
                <Button variant="outline" size="sm" className="w-full gap-2" onClick={() => handleChoose('withdraw')}>
                  <LogOut className="h-3.5 w-3.5" />
                  Withdraw (eliminate yourself)
                </Button>
              </div>
            </DialogContent>
          </Dialog>

          <Dialog open={showHistory} onOpenChange={setShowHistory}>
            <DialogContent className="sm:max-w-sm">
              <DialogHeader>
                <DialogTitle>Elimination History</DialogTitle>
              </DialogHeader>
              <div className="space-y-1.5">
                {rouletteState.eliminated.map((e) => (
                  <div key={e.player_id} className="flex items-center gap-2 rounded-md border px-2.5 py-1.5 text-sm">
                    <span className="font-medium text-muted-foreground">R{e.round}</span>
                    <span className="flex-1">{e.username}</span>
                    <Badge variant="outline" className="text-xs">
                      {e.reason === 'kicked' && e.by && `by ${e.by}`}
                      {e.reason === 'withdrew' && 'withdrew'}
                      {e.reason === 'timeout' && 'timeout'}
                    </Badge>
                  </div>
                ))}
              </div>
            </DialogContent>
          </Dialog>
        </div>
      )}
    </GamePageShell>
  );
}