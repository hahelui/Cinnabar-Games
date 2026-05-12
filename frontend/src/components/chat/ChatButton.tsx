import { MessageCircle } from 'lucide-react';
import { Button } from '@/components/ui/button';

interface ChatButtonProps {
  unread: boolean;
  onClick: () => void;
}

export function ChatButton({ unread, onClick }: ChatButtonProps) {
  return (
    <div className="fixed bottom-4 right-4 z-50">
      <Button
        variant="secondary"
        size="icon-lg"
        className="relative rounded-full bg-background/80 shadow-lg backdrop-blur-sm"
        onClick={onClick}
      >
        <MessageCircle />
        {unread && (
          <span className="absolute -top-0.5 -right-0.5 flex size-3">
            <span className="absolute inline-flex h-full w-full animate-ping rounded-full bg-red-400 opacity-75" />
            <span className="relative inline-flex size-3 rounded-full bg-red-500" />
          </span>
        )}
      </Button>
    </div>
  );
}
