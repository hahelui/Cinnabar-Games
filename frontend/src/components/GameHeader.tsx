import { useMemo, useState, type ReactNode } from 'react';
import { Link } from 'react-router';
import { useAuthStore } from '@/stores/authStore';
import { type RoomInfo, useGameStore } from '@/stores/gameStore';
import { Button } from '@/components/ui/button';
import { Input } from '@/components/ui/input';
import { Badge } from '@/components/ui/badge';
import { Dialog, DialogContent, DialogHeader, DialogTitle, DialogTrigger } from '@/components/ui/dialog';
import { Crown, LogOut, Settings, Users, ScrollText, Copy } from 'lucide-react';
import { toast } from 'sonner';

interface GameHeaderProps {
  room?: RoomInfo | null;
  rules?: string;
  onLeave?: () => void | Promise<void>;
  extraSettings?: ReactNode;
  dashboardOnly?: boolean;
}

export function GameHeader({ room, rules, onLeave, extraSettings, dashboardOnly = false }: GameHeaderProps) {
  const { player, login } = useAuthStore();
  const { kickPlayer, transferHost, setPresence } = useGameStore();
  const [name, setName] = useState(player?.username || '');
  const isHost = !!room && room.host_id === player?.player_id;
  const mode = useMemo(() => {
    if (!room || !player) return 'player';
    if (room.spectators?.some((p) => p.player_id === player.player_id)) return 'spectator';
    return 'player';
  }, [room, player]);
  const shareLink = room ? `${window.location.origin}/game/${room.game_type}/${room.room_id}` : '';
  const totalCount = room ? (room.players?.length || 0) + (room.spectators?.length || 0) : 0;

  const saveName = async () => {
    if (!name.trim() || name.trim() === player?.username) return;
    try {
      await login(name.trim());
    } catch {
      toast.error('Failed to update display name');
    }
  };

  const copyShare = async () => {
    if (!shareLink) return;
    await navigator.clipboard.writeText(shareLink);
    toast.success('Share link copied');
  };

  return (
    <header className="sticky top-0 z-50 border-b border-white/10 bg-transparent backdrop-blur-sm">
      <div className="mx-auto flex max-w-5xl items-center justify-between px-4 py-3">
        <div className="flex items-center gap-2">
          {!dashboardOnly && room && (
            <Dialog>
              <DialogTrigger asChild>
                <Button variant="secondary" className="gap-2">
                  <Users className="h-4 w-4" />
                  {totalCount}
                </Button>
              </DialogTrigger>
              <DialogContent>
                <DialogHeader>
                  <DialogTitle>Players</DialogTitle>
                </DialogHeader>
                <div className="max-h-72 space-y-4 overflow-y-auto">
                  <div className="space-y-2">
                    <div className="text-xs font-semibold text-muted-foreground">Players</div>
                    {room.players?.map((p) => (
                      <div key={p.player_id} className="flex items-center justify-between rounded-lg border px-2 py-1.5">
                        <div className="flex items-center gap-2">
                          <span>{p.username}</span>
                          {room.host_id === p.player_id && <Crown className="h-4 w-4 text-amber-500" />}
                        </div>
                        {isHost && room.host_id !== p.player_id && (
                          <div className="flex gap-1">
                            <Button size="xs" variant="outline" onClick={() => transferHost(p.player_id)}>
                              <Crown className="h-3 w-3" />
                            </Button>
                            <Button size="xs" variant="destructive" onClick={() => kickPlayer(p.player_id)}>
                              Kick
                            </Button>
                          </div>
                        )}
                      </div>
                    ))}
                  </div>
                  <div className="space-y-2">
                    <div className="text-xs font-semibold text-muted-foreground">Spectators</div>
                    {room.spectators?.map((p) => (
                      <div key={p.player_id} className="flex items-center justify-between rounded-lg border px-2 py-1.5">
                        <span>{p.username}</span>
                        {isHost && (
                          <Button size="xs" variant="destructive" onClick={() => kickPlayer(p.player_id)}>
                            Kick
                          </Button>
                        )}
                      </div>
                    ))}
                  </div>
                </div>
              </DialogContent>
            </Dialog>
          )}
          <Link to="/" className="text-lg font-bold tracking-tight">
            CinnabarGames
          </Link>
        </div>
        <div className="flex items-center gap-2">
          {!dashboardOnly && rules && (
            <Dialog>
              <DialogTrigger asChild>
                <Button variant="ghost" className="gap-2">
                  <ScrollText className="h-4 w-4" />
                  Rules
                </Button>
              </DialogTrigger>
              <DialogContent>
                <DialogHeader>
                  <DialogTitle>Rules</DialogTitle>
                </DialogHeader>
                <div className="max-h-80 overflow-y-auto whitespace-pre-wrap text-sm text-muted-foreground">{rules}</div>
              </DialogContent>
            </Dialog>
          )}
          <Dialog>
            <DialogTrigger asChild>
              <Button variant="ghost" size="icon">
                <Settings className="h-4 w-4" />
              </Button>
            </DialogTrigger>
            <DialogContent>
              <DialogHeader>
                <DialogTitle>Settings</DialogTitle>
              </DialogHeader>
              <div className="space-y-3">
                <div>
                  <div className="mb-1 text-xs text-muted-foreground">Display name</div>
                  <div className="flex gap-2">
                    <Input value={name} onChange={(e) => setName(e.target.value)} />
                    <Button variant="outline" onClick={saveName}>Save</Button>
                  </div>
                </div>
                {room && (
                  <>
                    <div>
                      <div className="mb-1 text-xs text-muted-foreground">Game code</div>
                      <Badge variant="secondary" className="font-mono">{room.room_id}</Badge>
                    </div>
                    <div>
                      <div className="mb-1 text-xs text-muted-foreground">Share link</div>
                      <div className="flex gap-2">
                        <Input value={shareLink} readOnly />
                        <Button variant="outline" size="icon" onClick={copyShare}>
                          <Copy className="h-4 w-4" />
                        </Button>
                      </div>
                    </div>
                    <div>
                      <div className="mb-1 text-xs text-muted-foreground">Side</div>
                      <div className="flex gap-2">
                        <Button variant={mode === 'player' ? 'default' : 'outline'} onClick={() => setPresence('player')}>Player</Button>
                        <Button variant={mode === 'spectator' ? 'default' : 'outline'} onClick={() => setPresence('spectator')}>Spectator</Button>
                      </div>
                    </div>
                    {extraSettings}
                  </>
                )}
                {!dashboardOnly && onLeave && (
                  <Button variant="destructive" className="mt-2 w-full gap-2" onClick={() => onLeave()}>
                    <LogOut className="h-4 w-4" />
                    Leave game
                  </Button>
                )}
              </div>
            </DialogContent>
          </Dialog>
        </div>
      </div>
    </header>
  );
}
