import { Outlet } from 'react-router';
import { GameHeader } from '@/components/GameHeader';

export function Layout() {
  return (
    <div className="flex min-h-svh flex-col">
      <GameHeader dashboardOnly />
      <main className="flex-1">
        <Outlet />
      </main>
      <footer className="border-t py-4 text-center text-xs text-muted-foreground">
        Cinnabar Games — Mini games for friends
      </footer>
    </div>
  );
}
