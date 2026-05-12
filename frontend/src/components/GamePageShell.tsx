import { type ReactNode } from 'react';
import { type RoomInfo, useGameStore } from '@/stores/gameStore';
import { GameHeader } from '@/components/GameHeader';
import { Badge } from '@/components/ui/badge';
import { useChat } from '@/hooks/useChat';
import { ChatButton } from '@/components/chat/ChatButton';
import { ChatPanel } from '@/components/chat/ChatPanel';

interface GamePageShellProps {
  room: RoomInfo | null;
  rules: string;
  onLeave: () => void | Promise<void>;
  title: string;
  statusText: string;
  badgeText?: string;
  badgeVariant?: 'default' | 'secondary' | 'destructive' | 'outline';
  maxWidthClassName?: string;
  children: ReactNode;
}

export function GamePageShell({
  room,
  rules,
  onLeave,
  title,
  statusText,
  badgeText,
  badgeVariant = 'secondary',
  maxWidthClassName = 'max-w-5xl',
  children,
}: GamePageShellProps) {
  const { chatOpen, chatUnread, toggleChat } = useGameStore();

  useChat(room?.room_id);
  if (!room) return null;

  return (
    <>
      <GameHeader room={room} rules={rules} onLeave={onLeave} />
      <div className={`mx-auto flex min-h-0 flex-1 flex-col ${maxWidthClassName} px-4 py-4`}>
        <div className="mb-3 flex items-center gap-3">
          <div className="flex-1">
            <h1 className="text-xl font-bold">{title}</h1>
            <p className="text-sm text-muted-foreground">{statusText}</p>
          </div>
          {badgeText && <Badge variant={badgeVariant}>{badgeText}</Badge>}
        </div>
        {children}
      </div>

      {chatOpen ? <ChatPanel onClose={toggleChat} /> : <ChatButton unread={chatUnread} onClick={toggleChat} />}
    </>
  );
}