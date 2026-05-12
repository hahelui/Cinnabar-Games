import { create } from 'zustand';
import { getNanoClient } from '@/services/nanoClient';
import { toast } from 'sonner';

export interface RoomPlayer {
  player_id: number;
  username: string;
}

export interface RoomInfo {
  room_id: string;
  game_type: string;
  host_id: number;
  host_name: string;
  status: string;
  players: RoomPlayer[];
  spectators: RoomPlayer[];
  config?: string;
}

export interface TTTState {
  board: string[];
  turn: number;
  winner: number;
  is_draw: boolean;
  players: number[];
  player_x: number;
  player_o: number;
  status: string;
}

export interface RPSState {
  round: number;
  max_rounds: number;
  scores: Record<string, number>;
  moves: Record<string, string>;
  revealed: boolean;
  winner: number;
  status: string;
  players: number[];
}

export interface RoulettePlayer {
  id: number;
  username: string;
}

export interface EliminationEntry {
  player_id: number;
  username: string;
  round: number;
  reason: string;
  by?: string;
}

export interface RouletteState {
  players: RoulettePlayer[];
  eliminated: EliminationEntry[];
  phase: string;
  round: number;
  selected_id: number;
  selected_index: number;
  spin_angle: number;
  spin_duration_ms: number;
  phase_ends_at: number;
  decision_deadline: number;
  last_event: string;
  winner: number;
  winner_name: string;
  status: string;
}

export interface MuamaraPlayer {
  id: number;
  username: string;
}

export interface MuamaraHistoryEntry {
  round: number;
  leader_id: number;
  leader_name: string;
  consultant_id: number;
  consultant_name: string;
  card_played: string;
  vote_result: string;
  votes?: Record<string, boolean>;
}

export interface MuamaraState {
  players: MuamaraPlayer[];
  dead_players: MuamaraPlayer[];
  phase: string;
  round: number;
  leader_id: number;
  consultant_id: number;
  recent_leaders: number[];
  recent_consultants: number[];
  votes_in: number;
  total_voters: number;
  vote_results?: Record<string, boolean>;
  failed_votes: number;
  force_approve: boolean;
  last_vote_approved: boolean;
red_cards_played: number;
  green_cards_played: number;
  revealed_card: string;
  card_count: number;
  active_special_power: string;
  history: MuamaraHistoryEntry[];
  status: string;
  winner: string;
  win_reason: string;
  last_event: string;
  player_count: number;
}

export interface MuamaraRolePayload {
  role: string;
  known_teammates: MuamaraPlayer[];
}

export interface MuamaraCardsPayload {
  cards: string[];
  phase: string;
}

export interface MuamaraInvestigatePayload {
  target_id: number;
  target_username: string;
  is_criminal_team: boolean;
}

export interface MafiaConfig {
  num_mafia: number;
  has_detective: boolean;
  has_doctor: boolean;
  has_sheriff: boolean;
  has_godfather: boolean;
  has_jester: boolean;
  reveal_on_death: boolean;
  announce_saved_player: boolean;
  night_timer_ms: number;
  discussion_timer_ms: number;
  vote_timer_ms: number;
}

export interface MafiaPlayer {
  id: number;
  username: string;
  alive: boolean;
  role?: string;
}

export interface MafiaVoteTally {
  player_id: number;
  username: string;
  vote_count: number;
  role?: string;
}

export interface MafiaState {
  players: MafiaPlayer[];
  dead_players: MafiaPlayer[];
  phase: string;
  round: number;
  config: MafiaConfig;
  phase_ends_at: number;
  night_ready_roles: string[];
  mafia_ready_count: number;
  last_night_kills: MafiaPlayer[];
  last_night_saved: boolean;
  last_night_event: string;
  votes_in: number;
  total_voters: number;
  vote_results: MafiaVoteTally[];
  is_revote: boolean;
  vote_candidates: number[];
  sheriff_used_ability: boolean;
  jester_won: boolean;
  status: string;
  winner: string;
  win_reason: string;
  last_event: string;
  host_id: number;
  history: { round: number; phase: string; target_id: number; target_name: string; detail: string }[];
  team_counts?: { mafia_alive: number; civ_alive: number; neutral_alive: number; total_dead: number };
}

export interface MafiaRolePayload {
  role: string;
  known_teammates: MafiaPlayer[];
}

export interface MafiaNightTargetsPayload {
  targets: MafiaPlayer[];
  role: string;
}

export interface MafiaNightResultPayload {
  killed_players: MafiaPlayer[];
  no_kill: boolean;
  sheriff_died: boolean;
  sheriff_killed: MafiaPlayer | null;
  saved_by_doctor: boolean;
}

export interface MafiaInvestigatePayload {
  target_id: number;
  target_username: string;
  result: string;
}

export interface MafiaMafiaMarksPayload {
  target_ids: number[];
}

export interface ChatTabDef {
  name: string;
  label: string;
  visibility: string;
  sendable_by: string;
  can_send: boolean;
}

export interface ChatMessage {
  player_id: number;
  username: string;
  content: string;
  tab: string;
  timestamp: number;
}

interface GameState {
  rooms: RoomInfo[];
  currentRoom: RoomInfo | null;
  tttState: TTTState | null;
  rpsState: RPSState | null;
  rouletteState: RouletteState | null;
muamaraState: MuamaraState | null;
  mafiaState: MafiaState | null;
  isLoading: boolean;
  isLeaving: boolean;
  chatTabs: ChatTabDef[];
  chatMessages: ChatMessage[];
  chatOpen: boolean;
  chatUnread: boolean;
  activeChatTab: string;

  listRooms: (gameType?: string) => Promise<void>;
  createRoom: (gameType: string, config?: string) => Promise<string | null>;
  joinRoom: (roomId: string, mode?: 'player' | 'spectator') => Promise<void>;
  getRoomInfo: (roomId: string) => Promise<RoomInfo | null>;
  leaveRoom: () => Promise<void>;
  startGame: () => Promise<void>;
  kickPlayer: (playerId: number) => Promise<void>;
  transferHost: (playerId: number) => Promise<void>;
  setPresence: (mode: 'player' | 'spectator') => Promise<void>;

  tttMove: (index: number) => Promise<void>;
  tttGetState: (roomId: string) => Promise<TTTState | null>;
  rpsPlay: (move: string) => Promise<void>;
  rpsGetState: (roomId: string) => Promise<RPSState | null>;
  rouletteChoose: (action: string, targetId?: number) => Promise<void>;
  rouletteGetState: (roomId: string) => Promise<RouletteState | null>;
  muamaraGetState: (roomId: string) => Promise<MuamaraState | null>;
  muamaraGetPrivateInfo: () => Promise<void>;
  muamaraSelectConsultant: (targetId: number) => Promise<void>;
  muamaraVote: (approve: boolean) => Promise<void>;
  muamaraEliminateCard: (cardIndex: number) => Promise<void>;
  muamaraUseSpecialPower: (targetId?: number) => Promise<void>;
  muamaraSkipPower: () => Promise<void>;
muamaraResetLeader: () => Promise<void>;

  mafiaGetState: (roomId: string) => Promise<MafiaState | null>;
  mafiaConfigure: (config: MafiaConfig) => Promise<void>;
  mafiaNightAction: (targetId: number, skip?: boolean) => Promise<boolean>;
  mafiaCastVote: (targetId: number) => Promise<void>;
  mafiaEndDiscussion: () => Promise<void>;
  mafiaSkipPhase: () => Promise<void>;

  setCurrentRoom: (room: RoomInfo | null) => void;
  setTTTState: (state: TTTState | null) => void;
  setRPSState: (state: RPSState | null) => void;
  setRouletteState: (state: RouletteState | null) => void;
  setMuamaraState: (state: MuamaraState | null) => void;
  setMafiaState: (state: MafiaState | null) => void;

  sendChatMessage: (tab: string, content: string) => Promise<void>;
  getChatMessages: (roomId: string) => Promise<void>;
  toggleChat: () => void;
  setActiveChatTab: (tab: string) => void;
  addChatMessage: (msg: ChatMessage) => void;

  addRoom: (room: RoomInfo) => void;
  updateRoom: (room: RoomInfo) => void;
  removeRoom: (roomId: string) => void;
}

let listRoomsTimer: ReturnType<typeof setTimeout> | null = null;
let createRoomPromise: Promise<string | null> | null = null;
let joinRoomPromise: Promise<void> | null = null;

export const useGameStore = create<GameState>((set, get) => ({
  rooms: [],
  currentRoom: null,
  tttState: null,
  rpsState: null,
  rouletteState: null,
muamaraState: null,
  mafiaState: null,
  isLoading: false,
  isLeaving: false,
  chatTabs: [],
  chatMessages: [],
  chatOpen: false,
  chatUnread: false,
  activeChatTab: 'general',

  listRooms: async (gameType = '') => {
    if (listRoomsTimer) return;
    set({ isLoading: true });
    try {
      const client = getNanoClient();
      const res = (await client.request('Lobby.ListRooms', { game_type: gameType })) as {
        rooms: RoomInfo[];
      };
      set({ rooms: res.rooms || [] });
    } catch {
      toast.error('Failed to list rooms');
    } finally {
      set({ isLoading: false });
      listRoomsTimer = setTimeout(() => { listRoomsTimer = null; }, 500);
    }
  },

  createRoom: async (gameType, config = '') => {
    if (createRoomPromise) return createRoomPromise;
    set({ isLoading: true });
    createRoomPromise = (async () => {
      try {
        const client = getNanoClient();
        const key = localStorage.getItem('cg_room_key') ?? undefined;
        const res = (await client.request('Lobby.CreateRoom', {
          game_type: gameType,
          config,
          key,
        })) as { code: number; message: string; room_id?: string };
        if (res.code !== 0) {
          toast.error(res.message || 'Failed to create room');
          return null;
        }
        toast.success('Room created!');
        await get().listRooms(gameType);
        return res.room_id || null;
      } catch {
        toast.error('Failed to create room');
        return null;
      } finally {
        set({ isLoading: false });
        createRoomPromise = null;
      }
    })();
    return createRoomPromise;
  },

  joinRoom: async (roomId, mode = 'player') => {
    if (joinRoomPromise) return joinRoomPromise;
    set({ isLoading: true });
    joinRoomPromise = (async () => {
      try {
        const client = getNanoClient();
        const res = (await client.request('Lobby.JoinRoom', { room_id: roomId, mode })) as {
          code: number;
          message: string;
        };
        if (res.code !== 0) {
          toast.error(res.message || 'Failed to join room');
          return;
        }
        const room = await get().getRoomInfo(roomId);
        if (room) {
          set({ currentRoom: room });
        }
        toast.success(mode === 'spectator' ? 'Joined as spectator' : 'Joined room');
      } catch {
        toast.error('Failed to join room');
      } finally {
        set({ isLoading: false });
        joinRoomPromise = null;
      }
    })();
    return joinRoomPromise;
  },

  getRoomInfo: async (roomId) => {
    try {
      const client = getNanoClient();
      const res = (await client.request('Lobby.GetRoomInfo', { room_id: roomId })) as
        | RoomInfo
        | { code: number; message: string };
      if ('code' in res) {
        toast.error(res.message || 'Failed to load room');
        return null;
      }
      return res;
    } catch {
      toast.error('Failed to load room');
      return null;
    }
  },

  leaveRoom: async () => {
    const room = get().currentRoom;
    if (!room || get().isLeaving) return;
    set({ isLeaving: true });
    try {
      const client = getNanoClient();
      await client.request('Lobby.LeaveRoom', { room_id: room.room_id });
    } catch {
      toast.error('Failed to leave room');
    }
    set({ currentRoom: null, tttState: null, rpsState: null, rouletteState: null, muamaraState: null, mafiaState: null, isLeaving: false, chatTabs: [], chatMessages: [], chatOpen: false, chatUnread: false, activeChatTab: 'general' });
  },

  startGame: async () => {
    const room = get().currentRoom;
    if (!room) return;
    try {
      const client = getNanoClient();
      const res = (await client.request('Lobby.StartGame', { room_id: room.room_id })) as {
        code: number;
        message: string;
      };
      if (res.code !== 0) {
        toast.error(res.message || 'Failed to start game');
      }
    } catch {
      toast.error('Failed to start game');
    }
  },

  kickPlayer: async (playerId) => {
    const room = get().currentRoom;
    if (!room) return;
    try {
      const client = getNanoClient();
      const res = (await client.request('Lobby.KickPlayer', {
        room_id: room.room_id,
        player_id: playerId,
      })) as { code: number; message: string };
      if (res.code !== 0) {
        toast.error(res.message || 'Failed to kick player');
        return;
      }
      toast.success('Player removed');
    } catch {
      toast.error('Failed to kick player');
    }
  },

  transferHost: async (playerId) => {
    const room = get().currentRoom;
    if (!room) return;
    try {
      const client = getNanoClient();
      const res = (await client.request('Lobby.TransferHost', {
        room_id: room.room_id,
        player_id: playerId,
      })) as { code: number; message: string };
      if (res.code !== 0) {
        toast.error(res.message || 'Failed to transfer host');
        return;
      }
      toast.success('Host transferred');
    } catch {
      toast.error('Failed to transfer host');
    }
  },

  setPresence: async (mode) => {
    const room = get().currentRoom;
    if (!room) return;
    try {
      const client = getNanoClient();
      const res = (await client.request('Lobby.SetPresence', {
        room_id: room.room_id,
        mode,
      })) as { code: number; message: string };
      if (res.code !== 0) {
        toast.error(res.message || 'Failed to update role');
        return;
      }
      toast.success(mode === 'spectator' ? 'Switched to spectator' : 'Switched to player');
    } catch {
      toast.error('Failed to update role');
    }
  },

  tttMove: async (index) => {
    const room = get().currentRoom;
    if (!room) return;
    try {
      const client = getNanoClient();
      await client.request('TicTacToe.MakeMove', { room_id: room.room_id, index });
    } catch (err) {
      toast.error(err instanceof Error ? err.message : 'Move failed');
    }
  },

  tttGetState: async (roomId) => {
    try {
      const client = getNanoClient();
      const res = await client.request('TicTacToe.GetState', { room_id: roomId });
      if (res && typeof res === 'object' && !('code' in res)) {
        return res as TTTState;
      }
    } catch {}
    return null;
  },

  rpsPlay: async (move) => {
    const room = get().currentRoom;
    if (!room) return;
    try {
      const client = getNanoClient();
      await client.request('RockPaperScissors.Play', { room_id: room.room_id, move });
    } catch (err) {
      toast.error(err instanceof Error ? err.message : 'Move failed');
    }
  },

  rpsGetState: async (roomId) => {
    try {
      const client = getNanoClient();
      const res = await client.request('RockPaperScissors.GetState', { room_id: roomId });
      if (res && typeof res === 'object' && !('code' in res)) {
        return res as RPSState;
      }
    } catch {}
    return null;
  },

  rouletteChoose: async (action, targetId) => {
    const room = get().currentRoom;
    if (!room) return;
    try {
      const client = getNanoClient();
      await client.request('Roulette.Choose', {
        room_id: room.room_id,
        action,
        target_id: targetId ?? 0,
      });
    } catch (err) {
      toast.error(err instanceof Error ? err.message : 'Action failed');
    }
  },

  rouletteGetState: async (roomId) => {
    try {
      const client = getNanoClient();
      const res = await client.request('Roulette.GetState', { room_id: roomId });
      if (res && typeof res === 'object' && !('code' in res)) {
        return res as RouletteState;
      }
    } catch {}
    return null;
  },

  muamaraGetState: async (roomId) => {
    try {
      const client = getNanoClient();
      const res = await client.request('AlMuamara.GetState', { room_id: roomId });
      if (res && typeof res === 'object' && !('code' in res)) {
        return res as MuamaraState;
      }
    } catch {}
    return null;
  },

  muamaraGetPrivateInfo: async () => {
    const room = get().currentRoom;
    if (!room) return;
    try {
      const client = getNanoClient();
      await client.request('AlMuamara.GetPrivateInfo', { room_id: room.room_id });
    } catch (err) {
      toast.error(err instanceof Error ? err.message : 'Failed to get private info');
    }
  },

  muamaraSelectConsultant: async (targetId) => {
    const room = get().currentRoom;
    if (!room) return;
    try {
      const client = getNanoClient();
      const res = (await client.request('AlMuamara.SelectConsultant', {
        room_id: room.room_id,
        target_id: targetId,
      })) as { code: number; message: string };
      if (res?.code !== undefined && res.code !== 0) {
        toast.error(res.message || 'Failed to select consultant');
      }
    } catch (err) {
      toast.error(err instanceof Error ? err.message : 'Action failed');
    }
  },

  muamaraVote: async (approve) => {
    const room = get().currentRoom;
    if (!room) return;
    try {
      const client = getNanoClient();
      const res = (await client.request('AlMuamara.CastVote', {
        room_id: room.room_id,
        approve,
      })) as { code: number; message: string };
      if (res?.code !== undefined && res.code !== 0) {
        toast.error(res.message || 'Vote failed');
      }
    } catch (err) {
      toast.error(err instanceof Error ? err.message : 'Vote failed');
    }
  },

  muamaraEliminateCard: async (cardIndex) => {
    const room = get().currentRoom;
    if (!room) return;
    try {
      const client = getNanoClient();
      const res = (await client.request('AlMuamara.EliminateCard', {
        room_id: room.room_id,
        card_index: cardIndex,
      })) as { code: number; message: string };
      if (res?.code !== undefined && res.code !== 0) {
        toast.error(res.message || 'Failed to eliminate card');
      }
    } catch (err) {
      toast.error(err instanceof Error ? err.message : 'Action failed');
    }
  },

  muamaraUseSpecialPower: async (targetId) => {
    const room = get().currentRoom;
    if (!room) return;
    try {
      const client = getNanoClient();
      const res = (await client.request('AlMuamara.UseSpecialPower', {
        room_id: room.room_id,
        target_id: targetId ?? 0,
      })) as { code: number; message: string };
      if (res?.code !== undefined && res.code !== 0) {
        toast.error(res.message || 'Special power failed');
      }
    } catch (err) {
      toast.error(err instanceof Error ? err.message : 'Action failed');
    }
  },

  muamaraSkipPower: async () => {
    const room = get().currentRoom;
    if (!room) return;
    try {
      const client = getNanoClient();
      await client.request('AlMuamara.SkipSpecialPower', { room_id: room.room_id });
    } catch (err) {
      toast.error(err instanceof Error ? err.message : 'Action failed');
    }
  },

  muamaraResetLeader: async () => {
    const room = get().currentRoom;
    if (!room) return;
    try {
      const client = getNanoClient();
      const res = (await client.request('AlMuamara.ResetLeaderSelection', {
        room_id: room.room_id,
      })) as { code: number; message: string };
      if (res?.code !== undefined && res.code !== 0) {
        toast.error(res.message || 'Failed to reset leader');
      }
    } catch (err) {
      toast.error(err instanceof Error ? err.message : 'Action failed');
    }
  },

  setCurrentRoom: (room) => set({ currentRoom: room, chatTabs: [], chatMessages: [], activeChatTab: 'general' }),
  setTTTState: (state) => set({ tttState: state }),
  setRPSState: (state) => set({ rpsState: state }),
  setRouletteState: (state) => set({ rouletteState: state }),
  setMuamaraState: (state) => set({ muamaraState: state }),
  setMafiaState: (state) => set({ mafiaState: state }),

  sendChatMessage: async (tab, content) => {
    const room = get().currentRoom;
    if (!room) return;
    try {
      const client = getNanoClient();
      await client.request('Chat.SendMessage', { room_id: room.room_id, tab, content });
    } catch {}
  },

  getChatMessages: async (roomId) => {
    try {
      const client = getNanoClient();
      const res = (await client.request('Chat.GetMessages', { room_id: roomId })) as {
        tabs: ChatTabDef[];
        messages: ChatMessage[];
      };
      if (res && 'tabs' in res) {
        set({ chatTabs: res.tabs, chatMessages: res.messages, activeChatTab: res.tabs[0]?.name ?? 'general' });
      }
    } catch {}
  },

  toggleChat: () => set((s) => ({ chatOpen: !s.chatOpen, chatUnread: false })),

  setActiveChatTab: (tab) => set({ activeChatTab: tab }),

  addChatMessage: (msg) =>
    set((s) => ({ chatMessages: [...s.chatMessages, msg], chatUnread: !s.chatOpen ? true : s.chatUnread })),

  mafiaGetState: async (roomId) => {
    try {
      const client = getNanoClient();
      const res = await client.request('Mafia.GetState', { room_id: roomId });
      if (res && typeof res === 'object' && !('code' in res)) {
        return res as MafiaState;
      }
    } catch {}
    return null;
  },

  mafiaConfigure: async (config) => {
    const room = get().currentRoom;
    if (!room) return;
    try {
      const client = getNanoClient();
      await client.request('Mafia.Configure', { room_id: room.room_id, config });
    } catch (err) {
      toast.error(err instanceof Error ? err.message : 'Configuration failed');
    }
  },

  mafiaNightAction: async (targetId, skip = false) => {
    const room = get().currentRoom;
    if (!room) return false;
    try {
      const client = getNanoClient();
      const res = (await client.request('Mafia.NightAction', {
        room_id: room.room_id,
        target_id: targetId,
        skip,
      })) as { code: number; message: string };
      if (res?.code !== undefined && res.code !== 0) {
        toast.error(res.message || 'Night action failed');
        return false;
      }
      return true;
    } catch (err) {
      toast.error(err instanceof Error ? err.message : 'Night action failed');
      return false;
    }
  },

  mafiaCastVote: async (targetId) => {
    const room = get().currentRoom;
    if (!room) return;
    try {
      const client = getNanoClient();
      const res = (await client.request('Mafia.CastVote', {
        room_id: room.room_id,
        target_id: targetId,
      })) as { code: number; message: string };
      if (res?.code !== undefined && res.code !== 0) {
        toast.error(res.message || 'Vote failed');
      }
    } catch (err) {
      toast.error(err instanceof Error ? err.message : 'Vote failed');
    }
  },

  mafiaEndDiscussion: async () => {
    const room = get().currentRoom;
    if (!room) return;
    try {
      const client = getNanoClient();
      await client.request('Mafia.EndDiscussion', { room_id: room.room_id });
    } catch (err) {
      toast.error(err instanceof Error ? err.message : 'Failed to end discussion');
    }
  },

  mafiaSkipPhase: async () => {
    const room = get().currentRoom;
    if (!room) return;
    try {
      const client = getNanoClient();
      await client.request('Mafia.SkipPhase', { room_id: room.room_id });
    } catch (err) {
      toast.error(err instanceof Error ? err.message : 'Failed to skip phase');
    }
  },
  addRoom: (room) => set((s) => ({ rooms: [...s.rooms, room] })),
  updateRoom: (room) =>
    set((s) => ({
      rooms: s.rooms.map((r) => (r.room_id === room.room_id ? room : r)),
      currentRoom: s.currentRoom?.room_id === room.room_id ? room : s.currentRoom,
    })),
  removeRoom: (roomId) =>
    set((s) => ({
      rooms: s.rooms.filter((r) => r.room_id !== roomId),
      currentRoom: s.currentRoom?.room_id === roomId ? null : s.currentRoom,
    })),
}));