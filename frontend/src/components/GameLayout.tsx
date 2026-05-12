import { Outlet } from 'react-router';

export function GameLayout() {
  return (
    <div className="flex h-svh flex-col overflow-hidden">
      <Outlet />
    </div>
  );
}