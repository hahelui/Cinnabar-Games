import { useEffect, useRef, useState } from 'react';
import { useNavigate } from 'react-router';
import { toast } from 'sonner';
import { useAuthStore } from '@/stores/authStore';
import { useGameStore, type RoomInfo } from '@/stores/gameStore';
import { getNanoClient } from '@/services/nanoClient';

interface UseGameRoomBootstrapOptions<T> {
  roomCode?: string;
  gameType: string;
  lobbyPath: string;
  gameUpdateEvent: string;
  setGameState: (state: T | null) => void;
  getStateHandler?: (roomId: string) => Promise<T | null>;
}

export function useGameRoomBootstrap<T>({
  roomCode,
  gameType,
  lobbyPath,
  gameUpdateEvent,
  setGameState,
  getStateHandler,
}: UseGameRoomBootstrapOptions<T>) {
  const navigate = useNavigate();
  const { player, isLoggedIn } = useAuthStore();
  const { joinRoom, updateRoom, setCurrentRoom, getRoomInfo } = useGameStore();
  const [canStart, setCanStart] = useState(false);
  const fetchedRef = useRef(false);

  useEffect(() => {
    if (!isLoggedIn) return;
    if (!roomCode) {
      navigate(lobbyPath, { replace: true });
      return;
    }

    let mounted = true;
    const bootstrap = async () => {
      const existing = useGameStore.getState().currentRoom;
      const alreadyIn =
        existing?.room_id === roomCode &&
        (existing.players?.some((p) => p.player_id === player?.player_id) ||
          existing.spectators?.some((p) => p.player_id === player?.player_id));

      if (!alreadyIn) {
        await joinRoom(roomCode);
        if (!mounted) return;
      }

      const room = useGameStore.getState().currentRoom;
      if (!room || room.room_id !== roomCode || room.game_type !== gameType) {
        navigate(lobbyPath, { replace: true });
        return;
      }

      setCanStart(
        room.host_id === player?.player_id &&
          (room.status === 'waiting' || room.status === 'finished')
      );

      if (getStateHandler && !fetchedRef.current && room.status !== 'waiting') {
        fetchedRef.current = true;
        const state = await getStateHandler(room.room_id);
        if (mounted && state) {
          setGameState(state);
        }
      }
    };

    bootstrap();

    const client = getNanoClient();
    const unsubs: (() => void)[] = [];

    const refreshCanStart = (room: RoomInfo) => {
      if (room.room_id !== roomCode) return;
      updateRoom(room);
      setCanStart(
        room.host_id === player?.player_id &&
          (room.status === 'waiting' || room.status === 'finished')
      );
    };

    unsubs.push(
      client.on('onRoomUpdated', (data) => {
        if (data && typeof data === 'object') {
          refreshCanStart(data as RoomInfo);
        }
      })
    );

    unsubs.push(
      client.on('onGameStarted', (data) => {
        if (data && typeof data === 'object') {
          const room = data as RoomInfo;
          if (room.room_id !== roomCode) return;
          updateRoom(room);
          setCanStart(false);
        }
      })
    );

    unsubs.push(
      client.on(gameUpdateEvent, (data) => {
        const state = data as T;
        setGameState(state);
        if (state && typeof state === 'object' && 'status' in state) {
          const gameStatus = (state as Record<string, unknown>).status;
          if (gameStatus === 'finished') {
            getRoomInfo(roomCode!).then((room) => {
              if (mounted && room) {
                updateRoom(room);
                setCanStart(
                  room.host_id === player?.player_id &&
                    (room.status === 'waiting' || room.status === 'finished')
                );
              }
            });
          }
        }
      })
    );

    unsubs.push(
      client.on('onKicked', () => {
        toast.error('You were kicked from the room');
        setCurrentRoom(null);
        setGameState(null);
        navigate(lobbyPath, { replace: true });
      })
    );

    return () => {
      mounted = false;
      unsubs.forEach((u) => u());
      setGameState(null);
    };
  }, [gameType, gameUpdateEvent, isLoggedIn, joinRoom, lobbyPath, navigate, player?.player_id, roomCode, setCurrentRoom, setGameState, updateRoom, getRoomInfo, getStateHandler]);

  return { canStart };
}