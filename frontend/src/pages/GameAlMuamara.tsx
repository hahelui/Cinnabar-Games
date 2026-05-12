import { useCallback, useEffect, useRef, useState } from 'react';
import { useNavigate, useParams } from 'react-router';
import { useAuthStore } from '@/stores/authStore';
import { useGameStore, type MuamaraCardsPayload, type MuamaraInvestigatePayload, type MuamaraRolePayload, type MuamaraState } from '@/stores/gameStore';
import { useGameRoomBootstrap } from '@/hooks/useGameRoomBootstrap';
import { GamePageShell } from '@/components/GamePageShell';
import { Button } from '@/components/ui/button';
import { Badge } from '@/components/ui/badge';
import {
  Dialog,
  DialogContent,
  DialogHeader,
  DialogTitle,
} from '@/components/ui/dialog';
import { getNanoClient } from '@/services/nanoClient';
import { toast } from 'sonner';
import {
  Users,
  RotateCcw,
  History,
  Crown,
  Eye,
  EyeOff,
  ShieldCheck,
  Skull,
  Vote,
  ChevronRight,
  Zap,
  Search,
} from 'lucide-react';

const MUAMARA_RULES = `
Al-Mu'amara (The Conspiracy) — 6-12 players

Two factions: Citizens vs Criminals. Citizens win by playing 5 green cards or executing the criminal. Criminals win by getting 6 red cards played.

Each round:
1. The leader selects a consultant.
2. All players vote to approve the team.
3. If approved: leader draws 3 cards, eliminates 1 (only leader sees). Consultant sees the 2 remaining, eliminates 1. The last card is revealed to all.
4. After every 2 consecutive rejections, a red card auto-plays.

Special powers (triggered by red cards):
• 2nd red: Investigation — leader secretly learns if a player is in the criminal faction.
• 4th red: Leader Selection — the current leader stays as leader.
• 5th red (9+ players): Execution — the leader permanently kills a player.
`;

const PHASE_LABELS: Record<string, string> = {
  starting: 'Game starting…',
  consultant_selection: 'Leader is choosing a consultant',
  voting: 'Team voting in progress',
  vote_result: 'Vote result',
  leader_card: 'Leader is examining cards',
  consultant_card: 'Consultant is examining cards',
  card_reveal: 'Card is being revealed',
  special_power: 'Special power active',
  game_over: 'Game over',
};

function CardBack() {
  return (
    <div className="flex h-20 w-14 items-center justify-center rounded-lg border-2 border-dashed border-muted-foreground/30 bg-muted/40">
      <div className="h-8 w-8 rounded-full border border-muted-foreground/20" />
    </div>
  );
}

function CardFace({ color, animate }: { color: string; animate?: boolean }) {
  const isRed = color === 'red';
  return (
    <div
      className={`flex h-20 w-14 items-center justify-center rounded-lg border-2 font-bold text-lg transition-all
        ${isRed ? 'border-red-500 bg-red-500/20 text-red-400' : 'border-emerald-500 bg-emerald-500/20 text-emerald-400'}
        ${animate ? 'animate-bounce scale-110' : ''}`}
    >
      {isRed ? '🔴' : '🟢'}
    </div>
  );
}

function ScoreCard({ label, value, max, color }: { label: string; value: number; max: number; color: string }) {
  return (
    <div className={`flex flex-col items-center rounded-lg border px-4 py-2 ${color}`}>
      <span className="text-xs font-medium uppercase tracking-wide opacity-70">{label}</span>
      <span className="text-2xl font-black">{value}<span className="text-base font-normal opacity-50">/{max}</span></span>
    </div>
  );
}

export function GameAlMuamara() {
  const navigate = useNavigate();
  const { roomCode } = useParams<{ roomCode: string }>();
  const { player } = useAuthStore();
  const {
    currentRoom,
    muamaraState,
    leaveRoom,
    startGame,
    muamaraGetState,
    muamaraGetPrivateInfo,
    muamaraSelectConsultant,
    muamaraVote,
    muamaraEliminateCard,
    muamaraUseSpecialPower,
    muamaraSkipPower,
    muamaraResetLeader,
    setMuamaraState,
  } = useGameStore();

  const { canStart } = useGameRoomBootstrap<MuamaraState>({
    roomCode,
    gameType: 'almuamara',
    lobbyPath: '/lobby/almuamara',
    gameUpdateEvent: 'onMuamaraUpdate',
    setGameState: setMuamaraState,
    getStateHandler: muamaraGetState,
  });

  const [myRole, setMyRole] = useState<MuamaraRolePayload | null>(null);
  const [myCards, setMyCards] = useState<MuamaraCardsPayload | null>(null);
  const [investigateResult, setInvestigateResult] = useState<MuamaraInvestigatePayload | null>(null);
  const [showRole, setShowRole] = useState(false);
  const [showHistory, setShowHistory] = useState(false);
  const [showInvestigate, setShowInvestigate] = useState(false);
  const [showVoteResult, setShowVoteResult] = useState(false);
  const [showPowerAnnounce, setShowPowerAnnounce] = useState(false);
  const [revealAnim, setRevealAnim] = useState(false);
  const prevPhaseRef = useRef('');

  const myId = player?.player_id;
  const isHost = currentRoom?.host_id === myId;
  const isLeader = muamaraState?.leader_id === myId;
  const isConsultant = muamaraState?.consultant_id === myId;

  useEffect(() => {
    const client = getNanoClient();
    const unsubs: (() => void)[] = [];

    unsubs.push(client.on('onMuamaraRole', (data) => {
      setMyRole(data as MuamaraRolePayload);
    }));

    unsubs.push(client.on('onMuamaraCards', (data) => {
      setMyCards(data as MuamaraCardsPayload);
    }));

    unsubs.push(client.on('onMuamaraInvestigate', (data) => {
      setInvestigateResult(data as MuamaraInvestigatePayload);
      setShowInvestigate(true);
    }));

    return () => unsubs.forEach((u) => u());
  }, []);

  useEffect(() => {
    if (!muamaraState) return;
    const phase = muamaraState.phase;

    if (phase === 'card_reveal' && prevPhaseRef.current !== 'card_reveal') {
      setRevealAnim(true);
      setTimeout(() => setRevealAnim(false), 1200);
    }

    if (phase === 'starting' || phase === 'consultant_selection') {
      setMyCards(null);
    }

    // show vote result dialog when entering vote_result phase
    if (phase === 'vote_result' && prevPhaseRef.current !== 'vote_result') {
      setShowVoteResult(true);
      setTimeout(() => setShowVoteResult(false), 5000);
    }

    // show special power announcement dialog when entering special_power phase
    if (phase === 'special_power' && prevPhaseRef.current !== 'special_power') {
      setShowPowerAnnounce(true);
      setTimeout(() => setShowPowerAnnounce(false), 4000);
    }

    prevPhaseRef.current = phase;
  }, [muamaraState?.phase]);

  const handleLeave = useCallback(async () => {
    await leaveRoom();
    navigate('/lobby/almuamara', { replace: true });
  }, [leaveRoom, navigate]);

  const handleGetPrivateInfo = useCallback(async () => {
    await muamaraGetPrivateInfo();
    setShowRole(true);
  }, [muamaraGetPrivateInfo]);

  const isAlive = useCallback((id: number) => {
    if (!muamaraState) return true;
    return !muamaraState.dead_players.some((p) => p.id === id);
  }, [muamaraState]);

  const aliveNonMePlayers = muamaraState?.players.filter(
    (p) => p.id !== myId && isAlive(p.id)
  ) ?? [];

  if (!currentRoom) return null;

  const state = muamaraState;
  const phase = state?.phase ?? '';

  const renderCardField = () => {
    if (!state) return null;

    const isLeaderCardPhase = phase === 'leader_card';
    const isConsultantCardPhase = phase === 'consultant_card';
    const isCardReveal = phase === 'card_reveal';
    const isGameOver = phase === 'game_over';

    if (!isLeaderCardPhase && !isConsultantCardPhase && !isCardReveal && !isGameOver && !state.revealed_card) {
      return null;
    }

    const showMyCards =
      (isLeaderCardPhase && isLeader && myCards?.phase === 'leader_card') ||
      (isConsultantCardPhase && isConsultant && myCards?.phase === 'consultant_card');

    const cardCount = state.card_count;

    return (
      <div className="mt-auto w-full border-t bg-card/50 px-4 pb-4 pt-3">
        <div className="mb-1 text-xs font-medium text-muted-foreground uppercase tracking-wide">
          Card Field
        </div>
        <div className="flex items-center justify-center gap-3">
          {isCardReveal || isGameOver ? (
            <div className="flex gap-3">
              <CardFace color={state.revealed_card} animate={revealAnim} />
            </div>
          ) : showMyCards && myCards ? (
            <div className="flex gap-3">
              {myCards.cards.map((c, i) => (
                <div key={i} className="flex flex-col items-center gap-1">
                  <CardFace color={c} />
                  {((isLeaderCardPhase && isLeader) || (isConsultantCardPhase && isConsultant)) && (
                    <Button
                      size="sm"
                      variant="outline"
                      className="h-6 px-2 text-xs"
                      onClick={() => muamaraEliminateCard(i)}
                    >
                      Discard
                    </Button>
                  )}
                </div>
              ))}
            </div>
          ) : (
            <div className="flex gap-3">
              {Array.from({ length: cardCount || 0 }).map((_, i) => (
                <CardBack key={i} />
              ))}
              {cardCount === 0 && <span className="text-sm text-muted-foreground">No cards in play</span>}
            </div>
          )}
        </div>
      </div>
    );
  };

  const renderActions = () => {
    if (!state) return null;

    if (phase === 'game_over') {
      return (
        <div className="flex flex-wrap justify-center gap-2">
          {canStart && (
            <Button onClick={startGame} size="sm" className="gap-1.5">
              <RotateCcw className="h-4 w-4" />
              Play Again
            </Button>
          )}
        </div>
      );
    }

    if (phase === 'consultant_selection' && isLeader) {
      return (
        <div className="space-y-2">
          <p className="text-center text-sm text-muted-foreground">Select a consultant:</p>
          <div className="flex flex-wrap justify-center gap-2">
            {aliveNonMePlayers.map((p) => (
              <Button
                key={p.id}
                variant="outline"
                size="sm"
                className="gap-1.5"
                onClick={() => muamaraSelectConsultant(p.id)}
              >
                <ChevronRight className="h-3.5 w-3.5" />
                {p.username}
              </Button>
            ))}
          </div>
        </div>
      );
    }

    if (phase === 'voting') {
      const hasVoted = state.vote_results
        ? String(myId) in state.vote_results
        : false;
      return (
        <div className="flex flex-col items-center gap-2">
          <p className="text-sm text-muted-foreground">
            {hasVoted ? 'Vote cast — waiting for others…' : 'Vote to approve or reject this team:'}
          </p>
          {!hasVoted && (
            <div className="flex gap-3">
              <Button
                onClick={() => muamaraVote(true)}
                className="gap-1.5 bg-emerald-600 hover:bg-emerald-700 text-white"
                size="sm"
              >
                <ShieldCheck className="h-4 w-4" />
                Approve
              </Button>
              <Button
                onClick={() => muamaraVote(false)}
                variant="destructive"
                size="sm"
                className="gap-1.5"
              >
                <Skull className="h-4 w-4" />
                Reject
              </Button>
            </div>
          )}
          <span className="text-xs text-muted-foreground">
            {state.votes_in}/{state.total_voters} voted
          </span>
        </div>
      );
    }

    if (phase === 'special_power' && isLeader) {
      const power = state.active_special_power;
      const needsTarget = power === 'investigation' || power === 'execution';

      return (
        <div className="space-y-2">
          <div className="text-center text-sm font-medium">
            {power === 'investigation' && <span className="flex items-center justify-center gap-1.5"><Search className="h-4 w-4" /> Investigation — select a player to investigate</span>}
            {power === 'leader_selection' && <span className="flex items-center justify-center gap-1.5"><Crown className="h-4 w-4" /> Leader Selection — stay as leader?</span>}
            {power === 'execution' && <span className="flex items-center justify-center gap-1.5"><Skull className="h-4 w-4" /> Execution — select a player to execute</span>}
          </div>
          {needsTarget ? (
            <div className="flex flex-wrap justify-center gap-2">
              {aliveNonMePlayers.map((p) => (
                <Button
                  key={p.id}
                  variant={power === 'execution' ? 'destructive' : 'outline'}
                  size="sm"
                  className="gap-1.5"
                  onClick={() => muamaraUseSpecialPower(p.id)}
                >
                  {power === 'execution' ? <Skull className="h-3.5 w-3.5" /> : <Search className="h-3.5 w-3.5" />}
                  {p.username}
                </Button>
              ))}
              <Button variant="ghost" size="sm" onClick={muamaraSkipPower}>
                Skip
              </Button>
            </div>
          ) : (
            <div className="flex justify-center gap-3">
              <Button
                size="sm"
                className="gap-1.5"
                onClick={() => muamaraUseSpecialPower()}
              >
                <Zap className="h-4 w-4" />
                Use Power
              </Button>
              <Button variant="ghost" size="sm" onClick={muamaraSkipPower}>
                Skip
              </Button>
            </div>
          )}
        </div>
      );
    }

    return null;
  };

  const renderPlayerList = () => {
    if (!state) return null;
    return (
      <div className="flex flex-wrap justify-center gap-2">
        {state.players.map((p) => {
          const isMe = p.id === myId;
          const isMeLeader = p.id === state.leader_id;
          const isMeConsultant = p.id === state.consultant_id;
          return (
            <div
              key={p.id}
              className={`flex items-center gap-1.5 rounded-full border px-3 py-1 text-sm
                ${isMe ? 'border-primary bg-primary/10' : 'border-border'}
                ${!isAlive(p.id) ? 'opacity-40 line-through' : ''}`}
            >
              {isMeLeader && <Crown className="h-3.5 w-3.5 text-amber-400" />}
              {isMeConsultant && <Vote className="h-3.5 w-3.5 text-sky-400" />}
              <span>{p.username}</span>
              {isMe && <span className="text-xs text-muted-foreground">(you)</span>}
            </div>
          );
        })}
        {state.dead_players.map((p) => (
          <div
            key={p.id}
            className="flex items-center gap-1.5 rounded-full border border-dashed border-muted px-3 py-1 text-sm opacity-40 line-through"
          >
            <Skull className="h-3.5 w-3.5" />
            <span>{p.username}</span>
          </div>
        ))}
      </div>
    );
  };

  const statusText = state ? (PHASE_LABELS[phase] ?? phase) : (currentRoom.status === 'waiting' ? 'Waiting for players' : '');

  return (
    <GamePageShell
      room={currentRoom}
      rules={MUAMARA_RULES}
      onLeave={handleLeave}
      title="Al-Mu'amara"
      statusText={statusText}
      badgeText={state ? `Round ${state.round}` : undefined}
      maxWidthClassName="max-w-2xl"
    >
      {currentRoom.status === 'waiting' && (
        <div className="flex flex-col items-center gap-3 py-8">
          <div className="flex items-center gap-2 text-sm text-muted-foreground">
            <Users className="h-4 w-4" />
            {currentRoom.players?.length || 0}/6-12 players
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

      {state && (
        <div className="flex min-h-0 flex-1 flex-col gap-3">
          {/* ── Section 1: Main info ── */}
          <div className="flex flex-col gap-2 rounded-xl border bg-card p-3">
            {/* Score row */}
            <div className="flex items-center justify-between gap-2">
              <ScoreCard
                label="Red"
                value={state.red_cards_played}
                max={6}
                color="border-red-500/30 bg-red-500/5 text-red-400"
              />
              <div className="flex flex-col items-center">
                <span className="text-xs text-muted-foreground">Round</span>
                <span className="text-3xl font-black">{state.round}</span>
              </div>
              <ScoreCard
                label="Green"
                value={state.green_cards_played}
                max={5}
                color="border-emerald-500/30 bg-emerald-500/5 text-emerald-400"
              />
            </div>

            {/* Phase + last event */}
            <div className="rounded-md bg-muted/40 px-3 py-1.5 text-center text-sm">
              {state.last_event || PHASE_LABELS[phase] || phase}
            </div>

            {/* Failed votes + history */}
            <div className="flex items-center justify-between text-xs text-muted-foreground">
              <span>
                Failed votes: <strong className={state.failed_votes >= 1 ? 'text-amber-400' : ''}>{state.failed_votes}/2</strong>
                {state.force_approve && (
                  <Badge variant="outline" className="ml-2 text-xs border-amber-400 text-amber-400">Force Approve</Badge>
                )}
              </span>
              <Button variant="ghost" size="icon" className="h-7 w-7" onClick={() => setShowHistory(true)}>
                <History className="h-4 w-4" />
              </Button>
            </div>

            {/* Player chips */}
            {renderPlayerList()}
          </div>

          {/* ── Section 2: Actions ── */}
          <div className="flex flex-col gap-2 rounded-xl border bg-card p-3">
            <div className="flex flex-wrap justify-center gap-2">
              <Button
                variant="outline"
                size="sm"
                className="gap-1.5"
                onClick={handleGetPrivateInfo}
              >
                {showRole ? <EyeOff className="h-3.5 w-3.5" /> : <Eye className="h-3.5 w-3.5" />}
                My Role
              </Button>
              {isHost && phase !== 'game_over' && (
                <Button
                  variant="outline"
                  size="sm"
                  className="gap-1.5 border-amber-400/50 text-amber-400"
                  onClick={() => {
                    muamaraResetLeader().catch(() => {});
                    toast.info('Leader reset requested');
                  }}
                >
                  <Crown className="h-3.5 w-3.5" />
                  Reset Leader
                </Button>
              )}
            </div>
            {renderActions()}
          </div>

          {/* ── Section 3: Card field ── */}
          {renderCardField()}

          {/* game over banner */}
          {phase === 'game_over' && (
            <div className={`rounded-xl border p-4 text-center ${state.winner === 'citizens' ? 'border-emerald-500 bg-emerald-500/10' : 'border-red-500 bg-red-500/10'}`}>
              <div className="text-2xl font-black">
                {state.winner === 'citizens' ? '🛡️ Citizens Win!' : '💀 Criminals Win!'}
              </div>
              <div className="mt-1 text-sm text-muted-foreground">{state.win_reason}</div>
            </div>
          )}
        </div>
      )}

      {/* Role modal */}
      <Dialog open={showRole} onOpenChange={setShowRole}>
        <DialogContent className="sm:max-w-sm">
          <DialogHeader>
            <DialogTitle>Your Role</DialogTitle>
          </DialogHeader>
          {myRole ? (
            <div className="space-y-3">
              <div className={`rounded-lg border p-4 text-center text-lg font-bold
                ${myRole.role === 'criminal' ? 'border-red-500 bg-red-500/10 text-red-400' :
                  myRole.role === 'assistant' ? 'border-orange-500 bg-orange-500/10 text-orange-400' :
                  'border-emerald-500 bg-emerald-500/10 text-emerald-400'}`}>
                {myRole.role === 'criminal' && <Skull className="mx-auto mb-1 h-6 w-6" />}
                {myRole.role === 'assistant' && <Zap className="mx-auto mb-1 h-6 w-6" />}
                {myRole.role === 'citizen' && <ShieldCheck className="mx-auto mb-1 h-6 w-6" />}
                {myRole.role.charAt(0).toUpperCase() + myRole.role.slice(1)}
              </div>
              {myRole.known_teammates.length > 0 && (
                <div>
                  <div className="mb-1 text-xs font-medium text-muted-foreground uppercase">Known allies:</div>
                  <div className="flex flex-wrap gap-1.5">
                    {myRole.known_teammates.map((t) => (
                      <Badge key={t.id} variant="outline" className="border-red-500/50 text-red-400">
                        {t.username}
                      </Badge>
                    ))}
                  </div>
                </div>
              )}
            </div>
          ) : (
            <p className="text-sm text-muted-foreground">Role info not available yet.</p>
          )}
        </DialogContent>
      </Dialog>

      {/* Investigation result modal */}
      <Dialog open={showInvestigate} onOpenChange={setShowInvestigate}>
        <DialogContent className="sm:max-w-sm">
          <DialogHeader>
            <DialogTitle>Investigation Result</DialogTitle>
          </DialogHeader>
          {investigateResult && (
            <div className={`rounded-lg border p-4 text-center
              ${investigateResult.is_criminal_team ? 'border-red-500 bg-red-500/10' : 'border-emerald-500 bg-emerald-500/10'}`}>
              <p className="font-bold text-lg">{investigateResult.target_username}</p>
              <p className={`mt-1 font-medium ${investigateResult.is_criminal_team ? 'text-red-400' : 'text-emerald-400'}`}>
                {investigateResult.is_criminal_team ? '🔴 Criminal Faction' : '🟢 Citizen Faction'}
              </p>
            </div>
          )}
        </DialogContent>
      </Dialog>

      {/* Vote result dialog */}
      <Dialog open={showVoteResult} onOpenChange={setShowVoteResult}>
        <DialogContent className="sm:max-w-md">
          <DialogHeader>
            <DialogTitle>Vote Result</DialogTitle>
          </DialogHeader>
          {state && (
            <div className="space-y-3">
              <div className={`rounded-lg border p-3 text-center text-lg font-bold
                ${state.last_vote_approved ? 'border-emerald-500 bg-emerald-500/10 text-emerald-400' : 'border-red-500 bg-red-500/10 text-red-400'}`}>
                {state.last_vote_approved ? '✅ Approved' : '❌ Rejected'}
              </div>
              {state.vote_results && (
                <div className="space-y-1">
                  {state.players.map((p) => {
                    const vote = state!.vote_results?.[String(p.id)];
                    if (vote === undefined || vote === null) return null;
                    return (
                      <div key={p.id} className="flex items-center justify-between rounded-md px-3 py-1.5 text-sm">
                        <span>{p.username}</span>
                        <span className={vote ? 'text-emerald-400 font-medium' : 'text-red-400 font-medium'}>
                          {vote ? 'Approve' : 'Reject'}
                        </span>
                      </div>
                    );
                  })}
                </div>
              )}
            </div>
          )}
        </DialogContent>
      </Dialog>

      {/* Special power announcement dialog */}
      <Dialog open={showPowerAnnounce} onOpenChange={setShowPowerAnnounce}>
        <DialogContent className="sm:max-w-sm">
          <DialogHeader>
            <DialogTitle>Special Power Activated!</DialogTitle>
          </DialogHeader>
          {state && (
            <div className="space-y-2 text-center">
              {state.active_special_power === 'investigation' && (
                <div className="rounded-lg border border-blue-500/30 bg-blue-500/10 p-4">
                  <Search className="mx-auto mb-2 h-8 w-8 text-blue-400" />
                  <p className="text-lg font-bold text-blue-400">Investigation</p>
                  <p className="mt-1 text-sm text-muted-foreground">
                    The leader can secretly reveal one player's faction.
                  </p>
                </div>
              )}
              {state.active_special_power === 'leader_selection' && (
                <div className="rounded-lg border border-amber-500/30 bg-amber-500/10 p-4">
                  <Crown className="mx-auto mb-2 h-8 w-8 text-amber-400" />
                  <p className="text-lg font-bold text-amber-400">Leader Selection</p>
                  <p className="mt-1 text-sm text-muted-foreground">
                    The current leader stays as leader for the next round.
                  </p>
                </div>
              )}
              {state.active_special_power === 'execution' && (
                <div className="rounded-lg border border-red-500/30 bg-red-500/10 p-4">
                  <Skull className="mx-auto mb-2 h-8 w-8 text-red-400" />
                  <p className="text-lg font-bold text-red-400">Execution</p>
                  <p className="mt-1 text-sm text-muted-foreground">
                    The leader can permanently eliminate one player!
                  </p>
                </div>
              )}
              {state.leader_id && (
                <p className="text-sm text-muted-foreground">
                  Leader: <strong>{state.players.find(p => p.id === state!.leader_id)?.username ?? 'Unknown'}</strong>
                </p>
              )}
            </div>
          )}
        </DialogContent>
      </Dialog>

      {/* History modal */}
      <Dialog open={showHistory} onOpenChange={setShowHistory}>
        <DialogContent className="sm:max-w-md">
          <DialogHeader>
            <DialogTitle>Round History</DialogTitle>
          </DialogHeader>
          <div className="max-h-80 space-y-2 overflow-y-auto">
            {state?.history.length === 0 && (
              <p className="text-sm text-muted-foreground text-center py-4">No rounds played yet.</p>
            )}
            {state?.history.map((h, i) => (
              <div key={i} className="flex items-center gap-3 rounded-lg border px-3 py-2 text-sm">
                <span className="font-medium text-muted-foreground shrink-0">R{h.round}</span>
                <div className="min-w-0 flex-1">
                  <span className="truncate">{h.leader_name} + {h.consultant_name}</span>
                </div>
                <div className={`shrink-0 h-4 w-4 rounded-full ${h.card_played === 'green' ? 'bg-emerald-500' : 'bg-red-500'}`} />
              </div>
            ))}
          </div>
        </DialogContent>
      </Dialog>
    </GamePageShell>
  );
}
