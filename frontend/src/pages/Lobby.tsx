import { useEffect, useState } from 'react';
import { useParams, useNavigate } from 'react-router';
import { useAuthStore } from '@/stores/authStore';
import { useGameStore } from '@/stores/gameStore';
import { getNanoClient } from '@/services/nanoClient';
import { Button } from '@/components/ui/button';
import { Card, CardContent } from '@/components/ui/card';
import { Badge } from '@/components/ui/badge';
import {
  Dialog,
  DialogContent,
  DialogHeader,
  DialogTitle,
  DialogDescription,
} from '@/components/ui/dialog';
import { Skeleton } from '@/components/ui/skeleton';
import {
  ArrowLeft,
  Plus,
  RefreshCw,
  LogIn,
  Eye,
  RotateCcw,
  Users,
  Crown,
  Copy,
  Check,
} from 'lucide-react';
import { toast } from 'sonner';

export function Lobby() {
  const { gameType } = useParams<{ gameType: string }>();
  const navigate = useNavigate();
  const { isLoggedIn, player } = useAuthStore();
  const {
    rooms,
    isLoading,
    listRooms,
    createRoom,
    joinRoom,
    setCurrentRoom,
    updateRoom,
    setTTTState,
    setRPSState,
    setRouletteState,
    setMuamaraState,
  } = useGameStore();

  const [showCreate, setShowCreate] = useState(false);
  const [copied, setCopied] = useState(false);

  useEffect(() => {
    if (!isLoggedIn) return;
    listRooms(gameType);

    const client = getNanoClient();
    const unsubs: (() => void)[] = [];

    unsubs.push(
      client.on('onRoomCreated', (data) => {
        if (data && typeof data === 'object') {
          const room = data as import('@/stores/gameStore').RoomInfo;
          setCurrentRoom(room);
        }
      })
    );
    unsubs.push(
      client.on('onRoomUpdated', (data) => {
        if (data && typeof data === 'object') {
          updateRoom(data as import('@/stores/gameStore').RoomInfo);
        }
      })
    );
    unsubs.push(
      client.on('onGameStarted', (data) => {
        if (data && typeof data === 'object') {
          const room = data as import('@/stores/gameStore').RoomInfo;
          updateRoom(room);
          navigate(`/game/${room.game_type}/${room.room_id}`);
        }
      })
    );
    unsubs.push(
      client.on('onTicTacToeUpdate', (data) => {
        setTTTState(data as ReturnType<typeof useGameStore.getState>['tttState']);
      })
    );
    unsubs.push(
      client.on('onRPSUpdate', (data) => {
        setRPSState(data as ReturnType<typeof useGameStore.getState>['rpsState']);
      })
    );
    unsubs.push(
      client.on('onRouletteUpdate', (data) => {
        setRouletteState(data as ReturnType<typeof useGameStore.getState>['rouletteState']);
      })
    );
    unsubs.push(
      client.on('onMuamaraUpdate', (data) => {
        setMuamaraState(data as ReturnType<typeof useGameStore.getState>['muamaraState']);
      })
    );

    return () => {
      unsubs.forEach((u) => u());
    };
  }, [isLoggedIn, gameType, listRooms, navigate, setCurrentRoom, setRPSState, setTTTState, setRouletteState, setMuamaraState, updateRoom]);

  const handleCreate = async () => {
    const roomCode = await createRoom(gameType || 'tictactoe');
    setShowCreate(false);
    if (roomCode) {
      navigate(`/game/${gameType}/${roomCode}`);
    }
  };

  const handleJoin = async (roomId: string, mode?: 'player' | 'spectator') => {
    await joinRoom(roomId, mode);
    const joinedRoom = useGameStore.getState().currentRoom;
    if (joinedRoom?.room_id === roomId) {
      navigate(`/game/${gameType}/${roomId}`);
    }
  };

  const handleCopy = async (id: string) => {
    const share = `${window.location.origin}/game/${gameType}/${id}`;
    await navigator.clipboard.writeText(share);
    setCopied(true);
    setTimeout(() => setCopied(false), 1500);
    toast.success('Share link copied');
  };

  const isKnownPlayer = (room: import('@/stores/gameStore').RoomInfo) => {
    if (!player) return false;
    return room.players.some((p) => p.player_id === player.player_id);
  };

  const gameLabel =
    gameType === 'rps' ? 'Rock Paper Scissors' :
    gameType === 'roulette' ? 'Roulette' :
    gameType === 'almuamara' ? "Al-Mu'amara" :
    gameType === 'mafia' ? 'Mafia' :
    'Tic Tac Toe';

  return (
    <div className="mx-auto max-w-3xl px-4 py-6">
      <div className="mb-6 flex items-center gap-3">
        <Button variant="ghost" size="icon" onClick={() => navigate('/')}>
          <ArrowLeft className="h-5 w-5" />
        </Button>
        <div>
          <h1 className="text-2xl font-bold">{gameLabel}</h1>
          <p className="text-sm text-muted-foreground">Lobby</p>
        </div>
      </div>

      <div className="mb-4 flex items-center gap-2">
        <Button onClick={() => setShowCreate(true)} className="gap-2">
          <Plus className="h-4 w-4" />
          Create Room
        </Button>
        <Button variant="outline" onClick={() => listRooms(gameType)} className="gap-2">
          <RefreshCw className={`h-4 w-4 ${isLoading ? 'animate-spin' : ''}`} />
          Refresh
        </Button>
      </div>

      {isLoading && rooms.length === 0 ? (
        <div className="space-y-3">
          <Skeleton className="h-24 w-full" />
          <Skeleton className="h-24 w-full" />
        </div>
      ) : rooms.length === 0 ? (
        <Card className="py-12 text-center">
          <CardContent>
            <p className="text-muted-foreground">No rooms available. Create one!</p>
          </CardContent>
        </Card>
      ) : (
        <div className="space-y-3">
          {rooms.map((room) => (
            <Card key={room.room_id} className="overflow-hidden">
              <CardContent className="flex items-center gap-4 p-4">
                <div className="min-w-0 flex-1">
                  <div className="mb-1 flex items-center gap-2">
                    <span className="font-mono text-xs text-muted-foreground truncate">
                      {room.room_id}
                    </span>
                    <Badge variant={room.status === 'waiting' ? 'secondary' : 'default'}>
                      {room.status}
                    </Badge>
                  </div>
                  <div className="flex items-center gap-3 text-sm text-muted-foreground">
                    <span className="flex items-center gap-1">
                      <Crown className="h-3 w-3" />
                      {room.host_name || 'Unknown'}
                    </span>
                    <span className="flex items-center gap-1">
                      <Users className="h-3 w-3" />
                      {(room.players?.length || 0) + (room.spectators?.length || 0)}
                    </span>
                  </div>
                </div>
                <div className="flex items-center gap-2">
                  <Button
                    variant="ghost"
                    size="icon"
                    onClick={() => handleCopy(room.room_id)}
                  >
                    {copied ? <Check className="h-4 w-4" /> : <Copy className="h-4 w-4" />}
                  </Button>
                  {room.status === 'waiting' && (
                    <Button size="sm" onClick={() => handleJoin(room.room_id)} className="gap-1">
                      <LogIn className="h-4 w-4" />
                      Join
                    </Button>
                  )}
                  {room.status !== 'waiting' && isKnownPlayer(room) && (
                    <Button size="sm" onClick={() => handleJoin(room.room_id, 'player')} className="gap-1">
                      <RotateCcw className="h-4 w-4" />
                      Rejoin
                    </Button>
                  )}
                  {room.status !== 'waiting' && !isKnownPlayer(room) && (
                    <Button size="sm" variant="outline" onClick={() => handleJoin(room.room_id, 'spectator')} className="gap-1">
                      <Eye className="h-4 w-4" />
                      Spectate
                    </Button>
                  )}
                </div>
              </CardContent>
            </Card>
          ))}
        </div>
      )}

      <Dialog open={showCreate} onOpenChange={setShowCreate}>
        <DialogContent>
          <DialogHeader>
            <DialogTitle>Create Room</DialogTitle>
            <DialogDescription>Create a new {gameLabel} room.</DialogDescription>
          </DialogHeader>
          <div className="flex justify-end gap-2">
            <Button variant="outline" onClick={() => setShowCreate(false)}>
              Cancel
            </Button>
            <Button onClick={handleCreate} disabled={isLoading}>
              Create
            </Button>
          </div>
        </DialogContent>
      </Dialog>
    </div>
  );
}
