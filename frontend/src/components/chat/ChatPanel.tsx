import { useRef, useEffect, useState, useCallback } from 'react';
import { X, Send, ChevronDown } from 'lucide-react';
import { useGameStore, type ChatMessage } from '@/stores/gameStore';
import { useAuthStore } from '@/stores/authStore';
import { Button } from '@/components/ui/button';

interface ChatPanelProps {
  onClose: () => void;
}

export function ChatPanel({ onClose }: ChatPanelProps) {
  const { player } = useAuthStore();
  const { chatTabs, chatMessages, activeChatTab, sendChatMessage, setActiveChatTab } = useGameStore();
  const [input, setInput] = useState('');
  const listRef = useRef<HTMLDivElement>(null);
  const [atBottom, setAtBottom] = useState(true);
  const atBottomRef = useRef(true);
  const [showNewMsgHint, setShowNewMsgHint] = useState(false);
  const newMsgTimerRef = useRef<ReturnType<typeof setTimeout>>(null);

  const filtered = chatMessages.filter((m) => m.tab === activeChatTab);
  const activeTab = chatTabs.find((t) => t.name === activeChatTab);
  const canSend = activeTab?.can_send ?? false;

  const scrollToBottom = useCallback(() => {
    if (listRef.current) {
      listRef.current.scrollTop = listRef.current.scrollHeight;
    }
    atBottomRef.current = true;
    setShowNewMsgHint(false);
    if (newMsgTimerRef.current) {
      clearTimeout(newMsgTimerRef.current);
      newMsgTimerRef.current = null;
    }
  }, []);

  useEffect(() => {
    if (atBottomRef.current && listRef.current) {
      listRef.current.scrollTop = listRef.current.scrollHeight;
    } else if (filtered.length > 0) {
      setShowNewMsgHint(true);
      if (newMsgTimerRef.current) clearTimeout(newMsgTimerRef.current);
      newMsgTimerRef.current = setTimeout(() => setShowNewMsgHint(false), 4000);
    }
  }, [filtered.length]);

  useEffect(() => {
    return () => {
      if (newMsgTimerRef.current) clearTimeout(newMsgTimerRef.current);
    };
  }, []);

  const handleScroll = () => {
    const el = listRef.current;
    if (!el) return;
    const isAtBottom = el.scrollHeight - el.scrollTop - el.clientHeight < 40;
    atBottomRef.current = isAtBottom;
    setAtBottom(isAtBottom);
    if (isAtBottom) {
      setShowNewMsgHint(false);
      if (newMsgTimerRef.current) {
        clearTimeout(newMsgTimerRef.current);
        newMsgTimerRef.current = null;
      }
    }
  };

  const handleSend = () => {
    const text = input.trim();
    if (!text || !canSend) return;
    sendChatMessage(activeChatTab, text);
    setInput('');
  };

  const handleKeyDown = (e: React.KeyboardEvent) => {
    if (e.key === 'Enter' && !e.shiftKey) {
      e.preventDefault();
      handleSend();
    }
  };

  return (
    <>
      {/* Backdrop for click-outside close */}
      <div className="fixed inset-0 z-40" onClick={onClose} />

      <div className="fixed bottom-20 right-4 z-50 flex w-80 flex-col rounded-lg border bg-background shadow-xl">
        {/* Header */}
        <div className="flex items-center justify-between border-b px-3 py-2">
          <span className="text-sm font-semibold">Chat</span>
          <Button variant="ghost" size="icon-xs" onClick={onClose}>
            <X className="size-3.5" />
          </Button>
        </div>

        {/* Tabs */}
        {chatTabs.length > 1 && (
          <div className="flex gap-0 border-b px-2">
            {chatTabs.map((tab) => (
              <button
                key={tab.name}
                className={`px-3 py-1.5 text-xs font-medium transition-colors ${
                  activeChatTab === tab.name
                    ? 'border-b-2 border-primary text-primary'
                    : 'text-muted-foreground hover:text-foreground'
                }`}
                onClick={() => setActiveChatTab(tab.name)}
              >
                {tab.label}
              </button>
            ))}
          </div>
        )}

        {/* Messages */}
        <div className="relative flex-1" style={{ maxHeight: '500px', minHeight: '200px' }}>
          <div
            ref={listRef}
            onScroll={handleScroll}
            className="absolute inset-0 space-y-1 overflow-y-auto px-3 py-2"
          >
            {filtered.length === 0 && (
              <p className="pt-8 text-center text-xs text-muted-foreground">No messages yet</p>
            )}
            {filtered.map((msg, i) => (
              <ChatBubble key={i} msg={msg} isOwn={msg.player_id === player?.player_id} />
            ))}
          </div>

          {/* New messages hint overlay */}
          {showNewMsgHint && !atBottom && (
            <div className="pointer-events-none absolute bottom-0 left-0 right-0 bg-gradient-to-t from-background/90 to-transparent px-3 py-4 text-center text-xs text-muted-foreground">
              New messages below
            </div>
          )}

          {/* Scroll-to-bottom button */}
          {!atBottom && (
            <div className="absolute bottom-1 right-2">
              <Button
                variant="secondary"
                size="icon-xs"
                className="size-7 rounded-full shadow-md"
                onClick={scrollToBottom}
              >
                <ChevronDown className="size-3.5" />
              </Button>
            </div>
          )}
        </div>

        {/* Input */}
        <div className="flex items-center gap-2 border-t px-3 py-2">
          <input
            className="flex-1 rounded-md border bg-muted px-2.5 py-1.5 text-sm outline-none placeholder:text-muted-foreground focus:border-primary"
            placeholder={canSend ? 'Type a message...' : 'You cannot send here'}
            value={input}
            onChange={(e) => setInput(e.target.value)}
            onKeyDown={handleKeyDown}
            disabled={!canSend}
            maxLength={500}
          />
          <Button variant="default" size="icon" onClick={handleSend} disabled={!input.trim() || !canSend}>
            <Send className="size-4" />
          </Button>
        </div>
      </div>
    </>
  );
}

function ChatBubble({ msg, isOwn }: { msg: ChatMessage; isOwn: boolean }) {
  const time = new Date(msg.timestamp).toLocaleTimeString([], { hour: '2-digit', minute: '2-digit' });
  return (
    <div className={`flex ${isOwn ? 'justify-end' : 'justify-start'}`}>
      <div
        className={`max-w-[80%] rounded-lg px-3 py-1.5 text-sm ${
          isOwn ? 'bg-primary text-primary-foreground' : 'bg-muted'
        }`}
      >
        {!isOwn && <p className="mb-0.5 text-[10px] font-semibold opacity-70">{msg.username}</p>}
        <p className="break-words">{msg.content}</p>
        <p className={`mt-0.5 text-[10px] ${isOwn ? 'text-primary-foreground/60' : 'text-muted-foreground'}`}>
          {time}
        </p>
      </div>
    </div>
  );
}
