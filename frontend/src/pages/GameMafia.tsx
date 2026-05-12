import { useCallback, useEffect, useRef, useState } from 'react';
import { useNavigate, useParams } from 'react-router';
import { useAuthStore } from '@/stores/authStore';
import { useGameStore, type MafiaConfig, type MafiaInvestigatePayload, type MafiaMafiaMarksPayload, type MafiaNightTargetsPayload, type MafiaRolePayload, type MafiaState } from '@/stores/gameStore';
import { useGameRoomBootstrap } from '@/hooks/useGameRoomBootstrap';
import { GamePageShell } from '@/components/GamePageShell';
import { Button } from '@/components/ui/button';
import { Badge } from '@/components/ui/badge';
import { Switch } from '@/components/ui/switch';
import {
  Dialog,
  DialogContent,
  DialogDescription,
  DialogHeader,
  DialogTitle,
} from '@/components/ui/dialog';
import { getNanoClient } from '@/services/nanoClient';
import { toast } from 'sonner';
import {
  Users,
  Skull,
  Eye,
  EyeOff,
  Crosshair,
  Heart,
  Vote,
  Crown,
  Swords,
  History,
  Moon,
  Sun,
  ChevronRight,
  Clock,
} from 'lucide-react';

const PHASE_LABELS: Record<string, string> = {
  setup: 'Setting up',
  night: 'Night Phase',
  night_result: 'Night Results',
  day_discussion: 'Day — Discussion',
  day_vote: 'Day — Voting',
  day_vote_result: 'Vote Results',
  game_over: 'Game Over',
};

const PHASE_ICONS: Record<string, string> = {
  night: '🌙',
  night_result: '🌅',
  day_discussion: '☀️',
  day_vote: '🗳️',
  day_vote_result: '📊',
  game_over: '🏆',
};

const ROLE_META: Record<string, { emoji: string; label: string; team: string; bg: string; border: string; text: string; description: string; ability: string; winCondition: string }> = {
  mafia:     { emoji: '🔴', label: 'Mafia',      team: 'Mafia',     bg: 'bg-red-500/10', border: 'border-red-500/30', text: 'text-red-400', description: 'A member of the Mafia who eliminates villagers at night.', ability: 'Each night, vote with fellow Mafia to kill one player.', winCondition: 'Outnumber the remaining Civilians.' },
  civilian:  { emoji: '⚪', label: 'Civilian',   team: 'Civilians', bg: 'bg-gray-500/10', border: 'border-gray-500/30', text: 'text-gray-300', description: 'An ordinary villager trying to survive.', ability: 'No special ability. Use discussion and voting to find the Mafia.', winCondition: 'Eliminate all Mafia members.' },
  detective: { emoji: '🔵', label: 'Detective',  team: 'Civilians', bg: 'bg-blue-500/10', border: 'border-blue-500/30', text: 'text-blue-400', description: 'A skilled investigator who can uncover the truth.', ability: 'Each night, investigate one player to learn if they are Mafia or Civilian.', winCondition: 'Eliminate all Mafia members.' },
  doctor:    { emoji: '💚', label: 'Doctor',      team: 'Civilians', bg: 'bg-green-500/10', border: 'border-green-500/30', text: 'text-green-400', description: 'A healer who protects the villagers from harm.', ability: 'Each night, protect one player from being killed. Cannot protect the same player twice in a row.', winCondition: 'Eliminate all Mafia members.' },
  sheriff:   { emoji: '⭐', label: 'Sheriff',     team: 'Civilians', bg: 'bg-yellow-500/10', border: 'border-yellow-500/30', text: 'text-yellow-400', description: 'A lawman with a single bullet — use it wisely.', ability: 'One-time night ability: shoot a player. Kills them if they are Mafia, otherwise you die.', winCondition: 'Eliminate all Mafia members.' },
  godfather: { emoji: '🖤', label: 'Godfather',  team: 'Mafia',     bg: 'bg-purple-500/10', border: 'border-purple-500/30', text: 'text-purple-400', description: 'The cunning leader of the Mafia who evades suspicion.', ability: 'Each night, vote with Mafia to kill. Appears as "Civilian" to the Detective.', winCondition: 'Outnumber the remaining Civilians.' },
  jester:    { emoji: '🃏', label: 'Jester',      team: 'Neutral',  bg: 'bg-orange-500/10', border: 'border-orange-500/30', text: 'text-orange-400', description: 'A trickster who wins by losing — get voted out to claim victory.', ability: 'No night action. Manipulate others into voting for you during the day.', winCondition: 'Be voted out by the town.' },
};

const DEFAULT_CONFIG: MafiaConfig = {
  num_mafia: 1,
  has_detective: false,
  has_doctor: true,
  has_sheriff: false,
  has_godfather: false,
  has_jester: false,
  reveal_on_death: true,
  announce_saved_player: true,
  night_timer_ms: 120000,
  discussion_timer_ms: 600000,
  vote_timer_ms: 90000,
};

export function GameMafia() {
  const navigate = useNavigate();
  const { roomCode } = useParams<{ roomCode: string }>();
  const { player } = useAuthStore();
  const {
    currentRoom,
    mafiaState,
    leaveRoom,
    startGame,
    mafiaGetState,
    mafiaConfigure,
    mafiaNightAction,
    mafiaCastVote,
    mafiaEndDiscussion,
    mafiaSkipPhase,
    setMafiaState,
  } = useGameStore();

  const { canStart } = useGameRoomBootstrap<MafiaState>({
    roomCode,
    gameType: 'mafia',
    lobbyPath: '/lobby/mafia',
    gameUpdateEvent: 'onMafiaUpdate',
    setGameState: setMafiaState,
    getStateHandler: mafiaGetState,
  });

  const [myRole, setMyRole] = useState<MafiaRolePayload | null>(null);
  const [showRole, setShowRole] = useState(false);
  const [showActionDialog, setShowActionDialog] = useState(false);
  const [nightTargets, setNightTargets] = useState<MafiaNightTargetsPayload | null>(null);
  const [nightActionDone, setNightActionDone] = useState(false);
  const [actionLoading, setActionLoading] = useState(false);
  const [mafiaMarks, setMafiaMarks] = useState<number[]>([]);
  const [investigateResult, setInvestigateResult] = useState<MafiaInvestigatePayload | null>(null);
  const [showInvestigate, setShowInvestigate] = useState(false);
  const [showHistory, setShowHistory] = useState(false);
  const [showVoteResult, setShowVoteResult] = useState(false);
  const [showGameOver, setShowGameOver] = useState(false);
  const [showSettings, setShowSettings] = useState(false);
  const [showVoteDialog, setShowVoteDialog] = useState(false);
  const [hasVoted, setHasVoted] = useState(false);
  const [config, setConfig] = useState<MafiaConfig>(DEFAULT_CONFIG);
  const [timeLeft, setTimeLeft] = useState(0);
  const timerRef = useRef<ReturnType<typeof setInterval> | null>(null);
  const prevPhaseRef = useRef('');

  const myId = player?.player_id ?? 0;
  const isHost = currentRoom?.host_id === myId;
  const state = mafiaState;

  useEffect(() => {
    const client = getNanoClient();
    const unsubs: (() => void)[] = [];
    unsubs.push(client.on('onMafiaRole', (data) => { setMyRole(data as MafiaRolePayload); }));
    unsubs.push(client.on('onMafiaNightTargets', (data) => { setNightTargets(data as MafiaNightTargetsPayload); setNightActionDone(false); setShowActionDialog(false); }));
    unsubs.push(client.on('onMafiaMarks', (data) => { setMafiaMarks((data as MafiaMafiaMarksPayload).target_ids); }));
    unsubs.push(client.on('onMafiaActionDone', () => { setNightActionDone(true); }));
    unsubs.push(client.on('onMafiaInvestigate', (data) => { setInvestigateResult(data as MafiaInvestigatePayload); setShowInvestigate(true); }));
    return () => {
      unsubs.forEach((u) => u());
      setMyRole(null);
      setNightTargets(null);
      setNightActionDone(false);
      setMafiaMarks([]);
      setInvestigateResult(null);
      setShowRole(false);
      setShowActionDialog(false);
      setShowVoteResult(false);
      setShowGameOver(false);
    };
  }, []);

  useEffect(() => {
    if (!state) return;
    if (state.phase === 'night') {
      setNightActionDone(false);
    } else {
      setNightTargets(null);
    }
    if (state.phase === 'night' && prevPhaseRef.current !== 'night') {
      setShowActionDialog(false);
    }
    if (state.phase === 'day_vote' && prevPhaseRef.current !== 'day_vote') {
      setHasVoted(false);
      setShowVoteDialog(false);
    }
    if (state.phase === 'day_vote_result' && prevPhaseRef.current !== 'day_vote_result') {
      setShowVoteResult(true);
    }
    if (state.phase === 'game_over' && prevPhaseRef.current !== 'game_over') {
      setShowGameOver(true);
    }
    if (state.phase === 'night_result' && prevPhaseRef.current !== 'night_result') {
      setShowVoteResult(false);
      setShowGameOver(false);
    }
    if (state.phase === 'day_discussion' && prevPhaseRef.current !== 'day_discussion') {
      setShowVoteResult(false);
      setShowGameOver(false);
    }
    prevPhaseRef.current = state.phase;
  }, [state?.phase]);

  useEffect(() => {
    if (state) return;
    const playerCount = currentRoom?.players?.length ?? 0;
    const maxMafia = playerCount <= 5 ? 1 : playerCount <= 7 ? 2 : playerCount <= 9 ? 3 : 4;
    if (config.num_mafia > maxMafia) {
      setConfig(prev => ({ ...prev, num_mafia: maxMafia }));
      toast.info(`Mafia count reduced to ${maxMafia} due to player count`);
    }
  }, [currentRoom?.players?.length]);

  useEffect(() => {
    if (timerRef.current) clearInterval(timerRef.current);
    timerRef.current = null;
    if (!state || state.phase === 'game_over' || state.phase === 'setup') {
      setTimeLeft(0);
      return;
    }
    const endsAt = state.phase_ends_at;
    if (!endsAt) { setTimeLeft(0); return; }

    const update = () => {
      const left = Math.max(0, Math.round((endsAt - Date.now()) / 1000));
      setTimeLeft(left);
    };
    update();
    timerRef.current = setInterval(update, 1000);
    return () => { if (timerRef.current) clearInterval(timerRef.current); };
  }, [state?.phase_ends_at, state?.phase]);

  const handleGetPrivateInfo = useCallback(() => { setShowRole((v) => !v); }, []);

  const handleLeave = useCallback(async () => {
    await leaveRoom();
    navigate('/lobby/mafia', { replace: true });
  }, [leaveRoom, navigate]);

  const playerRole = myRole?.role;
  const alivePlayers = state?.players.filter(p => p.alive) ?? [];
  const isAlive = state?.players.some(p => p.id === myId && p.alive) ?? true;
  const hasNightAction = playerRole && playerRole !== 'civilian' && playerRole !== 'jester' && !(playerRole === 'sheriff' && state?.sheriff_used_ability);

  const submitNightAction = async (targetId: number, skip?: boolean) => {
    setActionLoading(true);
    try {
      const ok = await mafiaNightAction(targetId, skip);
      if (ok) {
        setNightActionDone(true);
        setShowActionDialog(false);
      }
    } finally {
      setActionLoading(false);
    }
  };

  const formatTime = (s: number) => {
    if (s <= 0) return '0:00';
    const m = Math.floor(s / 60);
    const sec = s % 60;
    return `${m}:${sec.toString().padStart(2, '0')}`;
  };

  const timerPercent = () => {
    if (!state || !state.phase_ends_at) return 0;
    const total = state.phase === 'night' ? state.config.night_timer_ms
      : state.phase === 'day_discussion' ? state.config.discussion_timer_ms
      : state.phase === 'day_vote' ? state.config.vote_timer_ms
      : 5000;
    return Math.max(0, Math.min(100, (timeLeft / (total / 1000)) * 100));
  };

  const renderSetup = () => {
    if (state || currentRoom?.status !== 'waiting') return null;
    const playerCount = currentRoom.players?.length ?? 0;
    return (
      <div className="space-y-4">
        <div className="flex items-center justify-center gap-2 text-sm text-muted-foreground">
          <Users className="h-4 w-4" />
          {playerCount}/5-16 players needed
        </div>
        {isHost && (
          <div className="space-y-3">
            <Button variant="outline" size="sm" className="w-full gap-1.5" onClick={() => setShowSettings(true)}>
              <Crown className="h-3.5 w-3.5 text-amber-400" /> Change Settings
            </Button>
          </div>
        )}
        {canStart && playerCount >= 5 && (
          <Button onClick={async () => { await mafiaConfigure(config); startGame(); }} className="w-full gap-2" size="lg"><Swords className="h-5 w-5" /> Start Game</Button>
        )}
      </div>
    );
  };

  const renderSettingsDialog = () => {
    const playerCount = currentRoom?.players?.length ?? 0;
    const maxMafia = playerCount <= 5 ? 1 : playerCount <= 7 ? 2 : playerCount <= 9 ? 3 : 4;
    const roleSlots = [
      { key: 'has_detective' as const, label: 'Detective', emoji: '🔵', desc: 'Investigates one player each night' },
      { key: 'has_doctor' as const, label: 'Doctor', emoji: '💚', desc: 'Protects one player each night' },
      { key: 'has_sheriff' as const, label: 'Sheriff', emoji: '⭐', desc: 'Can shoot one player (one-time)' },
      { key: 'has_godfather' as const, label: 'Godfather', emoji: '🖤', desc: 'Appears as Civilian to Detective' },
      { key: 'has_jester' as const, label: 'Jester', emoji: '🃏', desc: 'Wins if voted out during the day' },
    ];
    return (
      <Dialog open={showSettings} onOpenChange={setShowSettings}>
        <DialogDescription className="sr-only">Game settings</DialogDescription>
        <DialogContent className="sm:max-w-md max-h-[85vh] overflow-y-auto">
          <DialogHeader>
            <DialogTitle className="flex items-center gap-2"><Crown className="h-4 w-4 text-amber-400" /> Game Settings</DialogTitle>
          </DialogHeader>
          <div className="space-y-4">
            <div className="space-y-2">
              <label className="text-xs font-medium text-muted-foreground uppercase">Mafia Members</label>
              <div className="flex items-center gap-3">
                <div className="flex gap-1">
                  {Array.from({ length: maxMafia }, (_, i) => (
                    <Button key={i + 1} size="sm" variant={config.num_mafia === i + 1 ? 'default' : 'outline'} className={`w-9 h-9 p-0 ${config.num_mafia === i + 1 ? 'bg-red-600 hover:bg-red-700 text-white' : ''}`} onClick={() => {
                        const next = { ...config, num_mafia: i + 1 };
                        if (next.num_mafia < 2) next.has_godfather = false;
                        setConfig(next);
                      }}>{i + 1}</Button>
                  ))}
                </div>
                <span className="text-xs text-muted-foreground">({maxMafia} max)</span>
              </div>
            </div>
            <div className="space-y-2">
              <label className="text-xs font-medium text-muted-foreground uppercase">Special Roles</label>
              {roleSlots.map(slot => {
                const disabled = slot.key === 'has_godfather' && (config.num_mafia < 2 || !config.has_detective);
                return (
                  <div key={slot.key} className={`flex items-center justify-between rounded-lg border px-3 py-2 ${disabled ? 'opacity-50' : ''}`}>
                    <div className="flex items-center gap-2">
                      <span className="text-lg">{slot.emoji}</span>
                      <div><div className="text-sm font-medium">{slot.label}</div><div className="text-[11px] text-muted-foreground">{slot.desc}{disabled && ' (Requires 2 Mafia + Detective)'}</div></div>
                    </div>
                    <Switch checked={config[slot.key] as boolean} disabled={disabled} onCheckedChange={(v) => {
                      const next = { ...config, [slot.key]: v };
                      if (slot.key === 'has_detective' && !v) next.has_godfather = false;
                      setConfig(next);
                    }} />
                  </div>
                );
              })}
            </div>
            {config.has_doctor && (
              <div className="flex items-center justify-between rounded-lg border px-3 py-2">
                <div><div className="text-sm font-medium">Announce saved player</div><div className="text-[11px] text-muted-foreground">Reveal who the doctor saved</div></div>
                <Switch checked={config.announce_saved_player} onCheckedChange={(v) => setConfig({ ...config, announce_saved_player: v })} />
              </div>
            )}
            <div className="flex items-center justify-between rounded-lg border px-3 py-2">
              <div><div className="text-sm font-medium">Reveal role on death</div><div className="text-[11px] text-muted-foreground">Show eliminated players' roles</div></div>
              <Switch checked={config.reveal_on_death} onCheckedChange={(v) => setConfig({ ...config, reveal_on_death: v })} />
            </div>
            <div className="space-y-2">
              <label className="text-xs font-medium text-muted-foreground uppercase flex items-center gap-1"><Clock className="h-3 w-3" /> Phase Timers</label>
              {[
                { label: 'Night', key: 'night_timer_ms' as const, options: [60000, 120000, 180000] },
                { label: 'Discussion', key: 'discussion_timer_ms' as const, options: [300000, 600000, 900000] },
                { label: 'Vote', key: 'vote_timer_ms' as const, options: [60000, 90000, 120000] },
              ].map(t => (
                <div key={t.key} className="flex items-center justify-between">
                  <span className="text-sm text-muted-foreground">{t.label}</span>
                  <div className="flex gap-1">
                    {t.options.map(ms => (
                      <Button key={ms} size="sm" variant={config[t.key] === ms ? 'default' : 'outline'} className={`h-7 px-2 text-[11px] ${config[t.key] === ms ? 'bg-blue-600 hover:bg-blue-700 text-white' : ''}`} onClick={() => setConfig({ ...config, [t.key]: ms })}>
                        {ms / 60000}m
                      </Button>
                    ))}
                  </div>
                </div>
              ))}
            </div>
            <Button size="sm" onClick={() => setShowSettings(false)} className="w-full">Done</Button>
          </div>
        </DialogContent>
      </Dialog>
    );
  };

  const renderTimer = () => {
    if (!state || state.phase === 'game_over' || timeLeft <= 0) return null;
    const pct = timerPercent();
    const isNight = state.phase === 'night';
    return (
      <div className="flex flex-col items-center gap-1">
        <div className="flex items-center gap-2">
          {isNight ? <Moon className="h-4 w-4 text-blue-300" /> : <Sun className="h-4 w-4 text-amber-300" />}
          <span className={`font-mono text-lg font-bold ${timeLeft <= 10 ? 'text-red-400 animate-pulse' : isNight ? 'text-blue-300' : 'text-amber-300'}`}>{formatTime(timeLeft)}</span>
        </div>
        <div className="h-1.5 w-full overflow-hidden rounded-full bg-muted">
          <div className={`h-full rounded-full transition-all duration-1000 ${isNight ? 'bg-blue-400' : 'bg-amber-400'}`} style={{ width: `${pct}%` }} />
        </div>
      </div>
    );
  };

  const renderNightReadyStatus = () => {
    if (!state || state.phase !== 'night') return null;
    const readyRoles = state.night_ready_roles ?? [];
    const cfg = state.config;
    const mafiaReady = state.mafia_ready_count ?? 0;

    const deadMafiaCount = cfg.reveal_on_death
      ? state.dead_players.filter(p => p.role === 'mafia' || p.role === 'godfather').length
      : 0;
    const totalMafiaSlots = Math.max(0, cfg.num_mafia - deadMafiaCount);

    const roleSlots: { name: string; used?: boolean }[] = [];
    if (cfg.has_detective) {
      const dead = cfg.reveal_on_death && state.dead_players.some(p => p.role === 'detective');
      if (!dead) roleSlots.push({ name: 'Detective' });
    }
    if (cfg.has_doctor) {
      const dead = cfg.reveal_on_death && state.dead_players.some(p => p.role === 'doctor');
      if (!dead) roleSlots.push({ name: 'Doctor' });
    }
    if (cfg.has_sheriff) {
      const dead = cfg.reveal_on_death && state.dead_players.some(p => p.role === 'sheriff');
      const used = state.sheriff_used_ability;
      if (!dead) roleSlots.push({ name: 'Sheriff', used });
    }

    return (
      <div className="flex flex-wrap justify-center gap-2">
        {totalMafiaSlots > 0 && (
          <div className="flex items-center gap-1.5 rounded-full border px-2.5 py-1 text-xs border-red-500/30 bg-red-500/10 text-red-400">
            <span className="flex gap-0.5">
              {Array.from({ length: totalMafiaSlots }, (_, i) => (
                <span key={i}>{i < mafiaReady ? '✅' : '⏳'}</span>
              ))}
            </span>
            <span>Mafia</span>
          </div>
        )}
        {roleSlots.map(r => {
          const isReady = readyRoles.includes(r.name);
          const isUsed = r.used;
          return (
            <div key={r.name} className={`flex items-center gap-1 rounded-full border px-2.5 py-1 text-xs ${isUsed ? 'border-muted-foreground/20 text-muted-foreground/50 bg-muted/20' : isReady ? 'border-emerald-500/50 text-emerald-400 bg-emerald-500/10' : 'border-muted-foreground/30 text-muted-foreground bg-muted/20'}`}>
              {isUsed ? '⊘' : isReady ? '✅' : '⏳'} {r.name}
            </div>
          );
        })}
      </div>
    );
  };

  const renderTeamCounts = () => {
    if (!state) return null;
    const totalAlive = state.players.length;
    const totalDead = state.dead_players.length;
    const tc = state.team_counts;

    if (tc) {
      return (
        <div className="flex flex-wrap justify-center gap-3">
          <div className="flex items-center gap-1">
            <span className="text-lg font-bold text-red-500">{tc.mafia_alive}</span>
            <span className="text-[11px] font-medium text-red-400/80">Mafia</span>
          </div>
          <div className="flex items-center gap-1">
            <span className="text-lg font-bold text-green-500">{tc.civ_alive}</span>
            <span className="text-[11px] font-medium text-green-400/80">Civilians</span>
          </div>
          {tc.neutral_alive > 0 && (
            <div className="flex items-center gap-1">
              <span className="text-lg font-bold text-purple-500">{tc.neutral_alive}</span>
              <span className="text-[11px] font-medium text-purple-400/80">Neutral</span>
            </div>
          )}
          <div className="flex items-center gap-1">
            <span className="text-lg font-bold text-gray-400">{tc.total_dead}</span>
            <span className="text-[11px] font-medium text-gray-400/80">Dead</span>
          </div>
        </div>
      );
    }

    return (
      <div className="flex flex-wrap justify-center gap-3">
        <div className="flex items-center gap-1">
          <span className="text-lg font-bold text-green-500">{totalAlive}</span>
          <span className="text-[11px] font-medium text-green-400/80">Alive</span>
        </div>
        {totalDead > 0 && (
          <div className="flex items-center gap-1">
            <span className="text-lg font-bold text-gray-400">{totalDead}</span>
            <span className="text-[11px] font-medium text-gray-400/80">Dead</span>
          </div>
        )}
      </div>
    );
  };

  const renderEventCard = () => {
    if (!state) return null;
    const msg = state.last_night_event;
    if (!msg) return null;

    const raw = msg;
    const eliminatedMatch = raw.match(/Eliminated:\s*([^$.]+)/);
    const savedMatch = raw.match(/([^$.]+was saved by a doctor[^$.]*)/i) || raw.match(/(Doctor successfully saved[^$.]*)/i);
    const sheriffDiedMatch = raw.match(/(The Sheriff misfired[^$.]*)/);
    const sheriffKillMatch = raw.match(/(The Sheriff shot[^$.]*)/);

    return (
      <div className="flex flex-col items-center gap-2 rounded-xl border bg-card p-3">
        <div className="text-xs font-medium uppercase tracking-wide text-muted-foreground">
          ☀️ Last Night
        </div>
        <div className="rounded-lg border border-muted-foreground/20 bg-muted/30 px-4 py-3 text-center text-sm w-full space-y-1">
          {eliminatedMatch && (
            <div className="text-red-400 font-bold">💀 {eliminatedMatch[1].trim()}</div>
          )}
          {savedMatch && (
            <div className="text-emerald-400 font-bold">🛡️ {savedMatch[1].trim()}</div>
          )}
          {sheriffDiedMatch && (
            <div className="text-amber-400 font-bold">⚠️ {sheriffDiedMatch[1].trim()}</div>
          )}
          {sheriffKillMatch && (
            <div className="text-yellow-400 font-bold">⭐ {sheriffKillMatch[1].trim()}</div>
          )}
          {!eliminatedMatch && !savedMatch && !sheriffDiedMatch && !sheriffKillMatch && (
            <div>{raw}</div>
          )}
        </div>
      </div>
    );
  };

  const renderVoteResultDialog = () => {
    if (!state || state.phase !== 'day_vote_result' || !state.vote_results || state.vote_results.length === 0) return null;
    const maxVotes = Math.max(...state.vote_results.map(v => v.vote_count));
    return (
      <Dialog open={showVoteResult} onOpenChange={setShowVoteResult}>
        <DialogDescription className="sr-only">Vote results for the current round</DialogDescription>
        <DialogContent className="sm:max-w-md">
          <DialogHeader>
            <DialogTitle>{state.is_revote ? '🔄 Revote Results' : '🗳️ Vote Results'}</DialogTitle>
          </DialogHeader>
          <div className="space-y-1.5">
            {state.vote_results.map(vr => {
              const pct = maxVotes > 0 ? (vr.vote_count / maxVotes) * 100 : 0;
              return (
                <div key={vr.player_id} className="relative flex items-center justify-between rounded-md border px-3 py-1.5 text-sm overflow-hidden">
                  <div className="absolute inset-0 bg-muted/30" style={{ width: `${pct}%` }} />
                  <span className="relative z-10 font-medium">{vr.username}</span>
                  <span className="relative z-10 font-bold tabular-nums">{vr.vote_count} {vr.vote_count === 1 ? 'vote' : 'votes'}</span>
                </div>
              );
            })}
          </div>
        </DialogContent>
      </Dialog>
    );
  };

  const renderGameOverDialog = () => {
    if (!state || state.phase !== 'game_over') return null;
    const isMafia = state.winner === 'mafia';
    const isJester = state.winner === 'jester';
    const playerMap = new Map<number, typeof state.dead_players[0]>();
    for (const p of state.dead_players ?? []) playerMap.set(p.id, p);
    for (const p of state.players) { if (!playerMap.has(p.id)) playerMap.set(p.id, p); }
    const allPlayers = Array.from(playerMap.values());
    const winBorder = isMafia ? 'border-red-500' : isJester ? 'border-orange-500' : 'border-emerald-500';
    const winBg = isMafia ? 'bg-red-500/10' : isJester ? 'bg-orange-500/10' : 'bg-emerald-500/10';
    const winText = isMafia ? 'text-red-400' : isJester ? 'text-orange-400' : 'text-emerald-400';

    return (
      <Dialog open={showGameOver} onOpenChange={setShowGameOver}>
        <DialogDescription className="sr-only">Game over results</DialogDescription>
        <DialogContent className="sm:max-w-lg max-h-[85vh] overflow-y-auto">
          <DialogHeader>
            <DialogTitle className={`text-center text-3xl font-black ${winText}`}>
              {isMafia ? '🔴 Mafia Wins!' : isJester ? '🃏 Jester Wins!' : '⚪ Civilians Win!'}
            </DialogTitle>
          </DialogHeader>

          {/* Winner Banner */}
          <div className={`rounded-xl border-2 ${winBorder} ${winBg} p-5 text-center`}>
            <div className="text-lg font-bold text-white">{state.win_reason}</div>
          </div>

          {/* All Players Grid */}
          <div className="space-y-2">
            <h3 className="text-sm font-bold uppercase tracking-wide text-muted-foreground">All Players</h3>
            <div className="grid grid-cols-2 gap-2">
              {allPlayers.map(p => {
                const r = p.role ?? 'civilian';
                const meta = ROLE_META[r] ?? ROLE_META.civilian;
                const isDead = !p.alive;
                return (
                  <div key={p.id} className={`flex items-center gap-2 rounded-lg border p-2.5 ${meta.border} ${meta.bg} ${isDead ? 'opacity-50' : ''}`}>
                    <span className="text-xl">{meta.emoji}</span>
                    <div className="flex-1 min-w-0">
                      <div className={`text-sm font-bold truncate ${meta.text} ${isDead ? 'line-through' : ''}`}>{p.username}</div>
                      <div className={`text-[11px] ${meta.text} opacity-80 ${isDead ? 'line-through' : ''}`}>{meta.label}</div>
                    </div>
                    {isDead && <Skull className="h-4 w-4 text-gray-400 shrink-0" />}
                  </div>
                );
              })}
            </div>
          </div>

          {/* Game Timeline */}
          {state.history.length > 0 && (
            <div className="space-y-2">
              <h3 className="text-sm font-bold uppercase tracking-wide text-muted-foreground">Timeline</h3>
              <div className="space-y-1.5 max-h-48 overflow-y-auto rounded-lg border bg-muted/20 p-2">
                {state.history.map((h, i) => (
                  <div key={i} className="text-xs text-muted-foreground border-l-2 border-muted-foreground/20 pl-2 py-0.5">
                    {h.detail}
                  </div>
                ))}
              </div>
            </div>
          )}
        </DialogContent>
      </Dialog>
    );
  };

  const renderVoteDialog = () => {
    if (!state || state.phase !== 'day_vote' || !isAlive) return null;
    const candidates = state.is_revote && state.vote_candidates
      ? alivePlayers.filter(p => state.vote_candidates.includes(p.id) && p.id !== myId)
      : alivePlayers.filter(p => p.id !== myId);
    return (
      <Dialog open={showVoteDialog} onOpenChange={setShowVoteDialog}>
        <DialogDescription className="sr-only">Cast your vote</DialogDescription>
        <DialogContent className="sm:max-w-sm">
          <DialogHeader>
            <DialogTitle>{state.is_revote ? '🔄 Revote' : '🗳️ Vote'}</DialogTitle>
          </DialogHeader>
          <div className="space-y-2">
            <p className="text-center text-sm text-muted-foreground">Select who to eliminate:</p>
            <div className="grid grid-cols-1 gap-2">
              {candidates.map(p => (
                <Button key={p.id} variant="destructive" size="sm" className="gap-1.5 w-full" onClick={() => { mafiaCastVote(p.id); setHasVoted(true); setShowVoteDialog(false); }}>
                  <Vote className="h-3.5 w-3.5" />{p.username}
                </Button>
              ))}
            </div>
            <p className="text-center text-xs text-muted-foreground">{state.votes_in}/{state.total_voters} voted</p>
          </div>
        </DialogContent>
      </Dialog>
    );
  };

  const renderNightActionDialog = () => {
    if (!state || state.phase !== 'night' || !isAlive || !hasNightAction || nightActionDone) return null;
    const role = playerRole!;
    const targets = nightTargets?.targets ?? [];
    const roleMeta = ROLE_META[role];
    return (
      <Dialog open={showActionDialog} onOpenChange={setShowActionDialog}>
        <DialogDescription className="sr-only">Choose your night action as {roleMeta?.label ?? role}</DialogDescription>
        <DialogContent className="sm:max-w-md">
          <DialogHeader>
            <DialogTitle className="flex items-center gap-2">
              {roleMeta?.emoji ?? '⚪'} {roleMeta?.label ?? role} — Night Action
            </DialogTitle>
          </DialogHeader>
          <div className="space-y-3">
            {role === 'mafia' || role === 'godfather' ? (
              <>
                <p className="text-sm text-muted-foreground text-center">Select a player to eliminate tonight.</p>
                <div className="grid grid-cols-2 gap-2">
                  {targets.map(p => {
                    const isMarked = mafiaMarks.includes(p.id);
                    return (
                      <Button key={p.id} variant="destructive" size="sm" className={`gap-1.5 w-full ${isMarked ? 'ring-2 ring-yellow-400' : ''}`} disabled={actionLoading} onClick={() => submitNightAction(p.id)}>
                        <Crosshair className="h-3.5 w-3.5" />{p.username}{isMarked && ' 🔴'}
                      </Button>
                    );
                  })}
                </div>
              </>
            ) : role === 'detective' ? (
              <>
                <p className="text-sm text-muted-foreground text-center">🕵️ Choose a player to investigate.</p>
                <div className="grid grid-cols-2 gap-2">
                  {targets.map(p => (
                    <Button key={p.id} variant="secondary" size="sm" className="gap-1.5 w-full" disabled={actionLoading} onClick={() => submitNightAction(p.id)}>
                      <Eye className="h-3.5 w-3.5" />{p.username}
                    </Button>
                  ))}
                </div>
              </>
            ) : role === 'doctor' ? (
              <>
                <p className="text-sm text-muted-foreground text-center">💚 Choose a player to protect tonight.</p>
                <div className="grid grid-cols-2 gap-2">
                  {targets.map(p => (
                    <Button key={p.id} variant="outline" size="sm" className="gap-1.5 w-full border-green-500/30 text-green-400 hover:bg-green-500/10" disabled={actionLoading} onClick={() => submitNightAction(p.id)}>
                      <Heart className="h-3.5 w-3.5" />{p.username}
                    </Button>
                  ))}
                </div>
              </>
            ) : role === 'sheriff' ? (
              <>
                <p className="text-sm text-muted-foreground text-center">⭐ Choose a player to shoot, or skip to save your ability.</p>
                <Button variant="outline" size="sm" className="w-full mb-2" disabled={actionLoading} onClick={() => submitNightAction(0, true)}>
                  Skip (save ability for later)
                </Button>
                <div className="grid grid-cols-2 gap-2">
                  {targets.map(p => (
                    <Button key={p.id} variant="destructive" size="sm" className="gap-1.5 w-full" disabled={actionLoading} onClick={() => submitNightAction(p.id)}>
                      <Crosshair className="h-3.5 w-3.5" />{p.username}
                    </Button>
                  ))}
                </div>
              </>
            ) : null}
          </div>
        </DialogContent>
      </Dialog>
    );
  };

  const roleMeta = playerRole ? ROLE_META[playerRole] : null;
  const teammates = myRole?.known_teammates ?? [];

  if (!currentRoom) return null;

  return (
    <GamePageShell
      room={currentRoom}
      rules={`MAFIA — Social Deduction

Win Conditions:
• Civilians (Civilian, Detective, Doctor, Sheriff): Eliminate all Mafia
• Mafia (Mafia, Godfather): Outnumber the Civilians
• Jester (Neutral): Get voted out during the day

Phases:
🌙 Night — Players with night abilities submit their actions
🌅 Night Results — See who was eliminated during the night
☀️ Day Discussion — Discuss who you suspect
🗳️ Day Vote — Vote to eliminate a player
📊 Vote Results — See the outcome of the vote

All Roles:
🔴 Mafia — Kills one player each night. Works with other Mafia members.
🖤 Godfather — Appears as Civilian when investigated by Detective.
⚪ Civilian — No special ability. Vote during the day to find Mafia.
🔵 Detective — Investigates one player each night to learn if they are Mafia or Civilian.
💚 Doctor — Protects one player each night from being killed.
⭐ Sheriff — One-time ability to shoot a player. Kills the target if they are Mafia, otherwise dies.
🃏 Jester — Wins if voted out during the day. Has no night action.

Settings:
• Reveal Role on Death: Off — roles of eliminated players are hidden until the game ends`}
      onLeave={handleLeave}
      title="Mafia"
      statusText={state ? PHASE_LABELS[state.phase] ?? state.phase : (currentRoom?.status === 'waiting' ? 'Waiting for players' : '')}
      badgeText={state ? `R${state.round}` : undefined}
      maxWidthClassName="max-w-2xl"
    >
      {currentRoom?.status === 'waiting' && renderSetup()}

      {state && (
        <div className="flex min-h-0 flex-1 flex-col gap-3">
          <div className="flex flex-col gap-2 rounded-xl border bg-card p-3">
            <div className="rounded-lg bg-muted/40 px-3 py-2 text-center text-sm font-medium">
              {PHASE_ICONS[state.phase] ?? ''} {state.last_event || PHASE_LABELS[state.phase] || state.phase}
            </div>
            {renderTimer()}
            {renderNightReadyStatus()}
            {state.phase !== 'setup' && renderTeamCounts()}
            <div className="flex flex-wrap justify-center gap-1.5">
              {state.players.map(p => {
                const meta = ROLE_META[p.role ?? 'civilian'] ?? ROLE_META.civilian;
                return (
                  <div key={p.id} className={`flex items-center gap-1 rounded-full border px-2.5 py-0.5 text-xs ${p.id === myId ? 'border-primary bg-primary/10 ring-1 ring-primary/30' : 'border-border'} ${!p.alive ? 'opacity-40 line-through' : ''}`}>
                    <span className="font-medium">{p.username}</span>
                    {p.role && state.config.reveal_on_death && !p.alive && <span className="text-[10px]">{meta.emoji}</span>}
                  </div>
                );
              })}
              {state.dead_players.filter(p => !state.players.some(sp => sp.id === p.id)).map(p => {
                const meta = ROLE_META[p.role ?? 'civilian'] ?? ROLE_META.civilian;
                return (
                  <div key={p.id} className="flex items-center gap-1 rounded-full border border-dashed border-muted px-2.5 py-0.5 text-xs opacity-40 line-through">
                    <Skull className="h-3 w-3" />
                    <span>{p.username}</span>
                    {p.role && state.config.reveal_on_death && <span>{meta.emoji}</span>}
                  </div>
                );
              })}
            </div>
          </div>

          <div className="flex flex-col gap-2 rounded-xl border bg-card p-3">
            <div className="flex flex-wrap justify-center gap-2">
              <Button variant="outline" size="sm" className="gap-1.5" onClick={handleGetPrivateInfo}>
                {showRole ? <EyeOff className="h-3.5 w-3.5" /> : <Eye className="h-3.5 w-3.5" />}
                My Role
              </Button>
              {state.phase !== 'game_over' && (
                <Button variant="ghost" size="icon" className="h-7 w-7" onClick={() => setShowHistory(true)}>
                  <History className="h-4 w-4" />
                </Button>
              )}
              {isHost && state.phase !== 'game_over' && (
                <Button variant="outline" size="sm" className="gap-1.5 border-amber-400/50 text-amber-400" onClick={() => mafiaSkipPhase()}>
                  <Crown className="h-3.5 w-3.5" />
                  Skip
                </Button>
              )}
            </div>

            {state.phase === 'night' && isAlive && hasNightAction && !nightActionDone && (
              <Button className="gap-2" onClick={() => setShowActionDialog(true)}>
                {roleMeta?.emoji ?? '⚪'} Take Action
                <ChevronRight className="h-4 w-4" />
              </Button>
            )}
            {state.phase === 'night' && isAlive && !hasNightAction && (
              <div className="text-center text-sm text-muted-foreground py-2">
                {playerRole === 'jester' ? '🃏 You have no night action. Wait for the day...' : '🌙 Wait for others to act...'}
              </div>
            )}
            {state.phase === 'night' && isAlive && nightActionDone && (
              <div className="text-center text-sm text-emerald-400 py-2">✅ Action submitted. Waiting for others...</div>
            )}
            {state.phase === 'night' && !isAlive && (
              <div className="text-center text-sm text-muted-foreground py-2">💀 You are dead. Watch from beyond...</div>
            )}
            {state.phase === 'day_discussion' && isHost && (
              <Button size="sm" className="w-full gap-1.5" onClick={() => mafiaEndDiscussion()}>
                <Vote className="h-3.5 w-3.5" /> End Discussion & Start Vote
              </Button>
            )}
            {state.phase === 'day_vote' && isAlive && !hasVoted && (
              <Button className="w-full gap-2" onClick={() => setShowVoteDialog(true)}>
                <Vote className="h-4 w-4" /> Cast Vote
              </Button>
            )}
            {state.phase === 'day_vote' && isAlive && hasVoted && (
              <div className="text-center text-sm text-emerald-400 py-2">✅ You have voted. Waiting for results...</div>
            )}
            {state.phase === 'day_vote' && !isAlive && (
              <div className="text-center text-sm text-muted-foreground py-2">💀 You are dead. Watching the vote...</div>
            )}
            {state.phase === 'game_over' && isHost && (
              <div className="space-y-2">
                <Button variant="outline" size="sm" className="w-full gap-1.5" onClick={() => setShowSettings(true)}>
                  <Crown className="h-3.5 w-3.5 text-amber-400" /> Change Settings
                </Button>
                <Button onClick={async () => { await mafiaConfigure(config); startGame(); }} size="sm" className="w-full gap-1.5">
                  <Swords className="h-3.5 w-3.5" /> Play Again
                </Button>
              </div>
            )}
            {state.phase === 'game_over' && !isHost && (
              <div className="text-center text-sm text-muted-foreground py-2">Waiting for host to start a new game...</div>
            )}
          </div>

          {renderEventCard()}
        </div>
      )}

      <Dialog open={showRole} onOpenChange={setShowRole}>
        <DialogDescription className="sr-only">Your secret role information</DialogDescription>
        <DialogContent className="sm:max-w-sm">
          <DialogHeader>
            <DialogTitle>Your Secret Role</DialogTitle>
          </DialogHeader>
          {myRole ? (
            <div className="space-y-3">
              <div className={`rounded-lg border p-4 text-center ${roleMeta?.bg ?? ''} ${roleMeta?.border ?? ''} ${roleMeta?.text ?? ''}`}>
                <div className="text-3xl mb-1">{roleMeta?.emoji ?? '⚪'}</div>
                <div className="text-xl font-bold">{roleMeta?.label ?? myRole.role}</div>
                <div className="text-xs opacity-70 mt-1">
                  {myRole.role === 'mafia' || myRole.role === 'godfather' ? 'Mafia Team' :
                   myRole.role === 'jester' ? 'Neutral Team' : 'Civilian Team'}
                </div>
              </div>

              {roleMeta && (
                <div className="space-y-2 text-sm">
                  <p className="text-muted-foreground">{roleMeta.description}</p>
                  <div className="rounded-lg border bg-muted/30 px-3 py-2">
                    <div className="text-xs font-semibold text-muted-foreground uppercase mb-0.5">Ability</div>
                    <p>{roleMeta.ability}</p>
                  </div>
                  <div className="rounded-lg border bg-muted/30 px-3 py-2">
                    <div className="text-xs font-semibold text-muted-foreground uppercase mb-0.5">Win Condition</div>
                    <p>{roleMeta.winCondition}</p>
                  </div>
                </div>
              )}

              {teammates.length > 0 && (
                <div>
                  <div className="mb-1 text-xs font-medium text-muted-foreground uppercase">Known allies:</div>
                  <div className="flex flex-wrap gap-1.5">
                    {teammates.map(t => (
                      <Badge key={t.id} variant="outline" className={`border-red-500/50 text-red-400 ${!t.alive ? 'opacity-50 line-through' : ''}`}>{t.username}{!t.alive ? ' 💀' : ''}</Badge>
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

      <Dialog open={showInvestigate} onOpenChange={setShowInvestigate}>
        <DialogDescription className="sr-only">Investigation result</DialogDescription>
        <DialogContent className="sm:max-w-sm">
          <DialogHeader>
            <DialogTitle>Investigation Result</DialogTitle>
          </DialogHeader>
          {investigateResult && (
            <div className={`rounded-lg border p-4 text-center ${investigateResult.result === 'mafia' ? 'border-red-500 bg-red-500/10' : 'border-emerald-500 bg-emerald-500/10'}`}>
              <p className="font-bold text-lg">{investigateResult.target_username}</p>
              <p className={`mt-1 text-lg font-bold ${investigateResult.result === 'mafia' ? 'text-red-400' : 'text-emerald-400'}`}>
                {investigateResult.result === 'mafia' ? '🔴 MAFIA' : '🟢 CIVILIAN'}
              </p>
            </div>
          )}
        </DialogContent>
      </Dialog>

      {renderVoteResultDialog()}
      {renderGameOverDialog()}
      {renderVoteDialog()}
      {renderNightActionDialog()}
      {renderSettingsDialog()}

      <Dialog open={showHistory} onOpenChange={setShowHistory}>
        <DialogDescription className="sr-only">Game event history</DialogDescription>
        <DialogContent className="sm:max-w-md">
          <DialogHeader>
            <DialogTitle>Game History</DialogTitle>
          </DialogHeader>
          <div className="max-h-80 space-y-2 overflow-y-auto">
            {!state?.history || state.history.length === 0 ? (
              <p className="text-sm text-muted-foreground text-center py-4">No game events yet.</p>
            ) : (
              state.history.map((h, i) => (
                <div key={i} className="flex items-center gap-3 rounded-lg border px-3 py-2 text-sm">
                  <span className="font-medium text-muted-foreground shrink-0">R{h.round}</span>
                  <span className="text-xs text-muted-foreground shrink-0">{h.phase === 'night' ? '🌙' : '☀️'}</span>
                  <div className="min-w-0 flex-1 truncate">{h.detail}</div>
                </div>
              ))
            )}
          </div>
        </DialogContent>
      </Dialog>
    </GamePageShell>
  );
}