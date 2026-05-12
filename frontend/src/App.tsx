import { BrowserRouter, Routes, Route, Navigate, useParams } from 'react-router';
import { Layout } from '@/components/Layout';
import { GameLayout } from '@/components/GameLayout';
import { AuthInitializer } from '@/components/AuthInitializer';
import { UsernameModal } from '@/components/UsernameModal';
import { Dashboard } from '@/pages/Dashboard';
import { Lobby } from '@/pages/Lobby';
import { GameTicTacToe } from '@/pages/GameTicTacToe';
import { GameRPS } from '@/pages/GameRPS';
import { GameRoulette } from '@/pages/GameRoulette';
import { GameAlMuamara } from '@/pages/GameAlMuamara';
import { GameMafia } from '@/pages/GameMafia';
import { Toaster } from '@/components/ui/sonner';

function GameRouteResolver() {
  const { gameType } = useParams<{ gameType: string }>();
  if (gameType === 'tictactoe') return <GameTicTacToe />;
  if (gameType === 'rps') return <GameRPS />;
  if (gameType === 'roulette') return <GameRoulette />;
  if (gameType === 'almuamara') return <GameAlMuamara />;
  if (gameType === 'mafia') return <GameMafia />;
  return <Navigate to="/" replace />;
}

export function App() {
  return (
    <BrowserRouter>
      <AuthInitializer />
      <UsernameModal />
      <Routes>
        <Route element={<Layout />}>
          <Route path="/" element={<Dashboard />} />
          <Route path="/lobby/:gameType" element={<Lobby />} />
        </Route>
        <Route element={<GameLayout />}>
          <Route path="/game/:gameType/:roomCode" element={<GameRouteResolver />} />
        </Route>
      </Routes>
      <Toaster position="top-center" />
    </BrowserRouter>
  );
}

export default App;