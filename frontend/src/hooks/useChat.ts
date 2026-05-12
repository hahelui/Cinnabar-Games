import { useEffect, useRef } from 'react';
import { useGameStore, type ChatMessage } from '@/stores/gameStore';
import { getNanoClient } from '@/services/nanoClient';

export function useChat(roomCode?: string) {
  const { getChatMessages, addChatMessage } = useGameStore();
  const fetchedRef = useRef(false);

  useEffect(() => {
    if (!roomCode) return;
    let mounted = true;

    const client = getNanoClient();

    if (!fetchedRef.current) {
      fetchedRef.current = true;
      getChatMessages(roomCode);
    }

    const unsubMsg = client.on('onChatMessage', (data) => {
      if (!mounted) return;
      const msg = data as ChatMessage;
      addChatMessage(msg);
    });

    // Re-fetch tabs when game starts — the server registers game-specific
    // tabs (e.g. "mafia") during InitGame, which happens after this hook
    // first runs on the waiting page.
    const unsubStart = client.on('onGameStarted', (data) => {
      if (!mounted) return;
      const room = data as { room_id?: string };
      if (room?.room_id === roomCode) {
        getChatMessages(roomCode);
      }
    });

    return () => {
      mounted = false;
      unsubMsg();
      unsubStart();
    };
  }, [roomCode, getChatMessages, addChatMessage]);
}
