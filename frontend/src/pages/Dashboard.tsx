import { useState } from 'react';
import { Link } from 'react-router';
import { Card, CardContent, CardHeader, CardTitle, CardDescription } from '@/components/ui/card';
import { Button } from '@/components/ui/button';
import { Input } from '@/components/ui/input';
import { Swords, Grid3x3, Users, ArrowRight, CircleDot, Shield, Key } from 'lucide-react';
import { useAuthStore } from '@/stores/authStore';

const games = [
  {
    id: 'tictactoe',
    name: 'Tic Tac Toe',
    description: 'Classic 2-player strategy game. First to align 3 wins!',
    icon: Grid3x3,
    players: '2 players',
    color: 'text-emerald-500',
    bg: 'bg-emerald-500/10',
  },
  {
    id: 'rps',
    name: 'Rock Paper Scissors',
    description: 'Quick reflexes! Best of 3 rounds against your friend.',
    icon: Swords,
    players: '2 players',
    color: 'text-amber-500',
    bg: 'bg-amber-500/10',
  },
  {
    id: 'roulette',
    name: 'Roulette',
    description: 'Elimination wheel! Get chosen, kick someone or withdraw. Last one standing wins.',
    icon: CircleDot,
    players: '3-20 players',
    color: 'text-rose-500',
    bg: 'bg-rose-500/10',
  },
{
    id: 'almuamara',
    name: 'Al-Mu\'amara',
    description: 'Social deduction: Citizens vs Criminals. Vote, play cards, and uncover the conspiracy.',
    icon: Shield,
    players: '6-12 players',
    color: 'text-violet-500',
    bg: 'bg-violet-500/10',
  },
  {
    id: 'mafia',
    name: 'Mafia',
    description: 'Classic social deduction: Mafia kills at night, Civilians vote by day. Find and eliminate the Mafia!',
    icon: Swords,
    players: '5-16 players',
    color: 'text-red-500',
    bg: 'bg-red-500/10',
  },
];

export function Dashboard() {
  const { isLoggedIn, player } = useAuthStore();
  const [showKeyInput, setShowKeyInput] = useState(false);
  const [keyValue, setKeyValue] = useState(localStorage.getItem('cg_room_key') ?? '');

  const saveKey = () => {
    if (keyValue.trim()) {
      localStorage.setItem('cg_room_key', keyValue.trim());
    } else {
      localStorage.removeItem('cg_room_key');
    }
    setShowKeyInput(false);
  };

  return (
    <div className="mx-auto max-w-5xl px-4 py-8">
      <div className="mb-8 flex flex-col gap-2">
        <div className="flex items-center justify-between">
          <div>
            <h1 className="text-3xl font-bold tracking-tight">
              {isLoggedIn ? `Hey, ${player?.username}!` : 'Welcome!'}
            </h1>
            <p className="text-muted-foreground">
              Pick a game, create a room, and invite your friends to play.
            </p>
          </div>
          <Button variant="outline" size="sm" onClick={() => setShowKeyInput(!showKeyInput)} className="gap-2">
            <Key className="h-4 w-4" />
            {localStorage.getItem('cg_room_key') ? 'Key set' : 'Set key'}
          </Button>
        </div>
        {showKeyInput && (
          <div className="flex items-center gap-2">
            <Input
              placeholder="Room creation key"
              value={keyValue}
              onChange={(e) => setKeyValue(e.target.value)}
              className="max-w-xs"
              maxLength={64}
            />
            <Button size="sm" onClick={saveKey}>Save</Button>
          </div>
        )}
      </div>

      <div className="grid gap-4 sm:grid-cols-2">
        {games.map((game) => {
          const Icon = game.icon;
          return (
            <Card key={game.id} className="group transition-colors hover:border-primary/50">
              <CardHeader className="flex flex-row items-center gap-4 pb-2">
                <div className={`flex h-12 w-12 items-center justify-center rounded-xl ${game.bg}`}>
                  <Icon className={`h-6 w-6 ${game.color}`} />
                </div>
                <div className="flex-1">
                  <CardTitle className="text-lg">{game.name}</CardTitle>
                  <CardDescription className="flex items-center gap-1 text-xs">
                    <Users className="h-3 w-3" />
                    {game.players}
                  </CardDescription>
                </div>
              </CardHeader>
              <CardContent className="flex flex-col gap-4">
                <p className="text-sm text-muted-foreground">{game.description}</p>
                <Button asChild className="w-full gap-2">
                  <Link to={`/lobby/${game.id}`}>
                    Play
                    <ArrowRight className="h-4 w-4 transition-transform group-hover:translate-x-0.5" />
                  </Link>
                </Button>
              </CardContent>
            </Card>
          );
        })}
      </div>
    </div>
  );
}
