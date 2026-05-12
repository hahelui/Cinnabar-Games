import { useState } from 'react';
import { useAuthStore } from '@/stores/authStore';
import {
  Dialog,
  DialogContent,
  DialogHeader,
  DialogTitle,
  DialogDescription,
} from '@/components/ui/dialog';
import { Input } from '@/components/ui/input';
import { Button } from '@/components/ui/button';
import { User } from 'lucide-react';

export function UsernameModal() {
  const { isLoggedIn, isAuthChecked, login, isConnecting } = useAuthStore();
  const [username, setUsername] = useState('');

  const handleSubmit = async (e: React.FormEvent) => {
    e.preventDefault();
    if (!username.trim()) return;
    await login(username.trim());
  };

  return (
    <Dialog open={isAuthChecked && !isLoggedIn} onOpenChange={() => {}}>
      <DialogContent className="sm:max-w-md" onPointerDownOutside={(e) => e.preventDefault()}>
        <DialogHeader>
          <DialogTitle className="flex items-center gap-2">
            <User className="h-5 w-5" />
            Welcome to Cinnabar Games
          </DialogTitle>
          <DialogDescription>
            Enter a display name to start playing with your friends.
          </DialogDescription>
        </DialogHeader>
        <form onSubmit={handleSubmit} className="flex flex-col gap-4">
          <Input
            placeholder="Your nickname"
            value={username}
            onChange={(e) => setUsername(e.target.value)}
            maxLength={14}
            autoFocus
          />
          <Button type="submit" disabled={isConnecting || !username.trim()}>
            {isConnecting ? 'Connecting...' : 'Join'}
          </Button>
        </form>
      </DialogContent>
    </Dialog>
  );
}