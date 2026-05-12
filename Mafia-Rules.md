# 🕵️ Mafia — Game Rules & Flow

## Overview

**Mafia** is a real-time social deduction party game where two factions compete: the **Mafia** (who know each other and kill in secret) and the **Civilians** (who must identify and eliminate the Mafia through discussion and voting).

The game is fully automated and runs in real-time over WebSockets. A designated **Host** manages the pacing of the game — they can advance or skip phases, but the game logic and role resolution are handled entirely by the system.

---

## Players & Roles

### Player Count & Mafia Scaling

| Total Players | Max Mafia |
|---------------|-----------|
| 5 | 1 |
| 6 – 7 | 2 |
| 8 – 9 | 3 |
| 10+ | 4 |

> The host sets the exact number of Mafia during setup, up to the allowed maximum for the player count.

---

### Core Roles

#### 🔴 Mafia
- Know each other's identities from the start of the game.
- Each night, they independently submit an elimination target via a private action dialogue.
- During the day, they pose as Civilians.

#### ⚪ Civilian
- No special abilities.
- Must use discussion, logic, and social deduction to identify and vote out Mafia members.
- **Default role** — all players not assigned a special role are Civilians.

---

### Special Roles

All special roles are **off by default**. The host can toggle each one on during the lobby/setup phase.

| Role | Faction | Ability | Default |
|------|---------|---------|---------|
| 🔵 **Detective** | Civilian | Each night, investigates one player. The system privately returns: *Mafia* or *Civilian*. | Off |
| 🟢 **Doctor** | Civilian | Each night, selects one player to protect from elimination. Cannot protect the same player two nights in a row (including themselves). May protect themselves multiple times across the game, as long as the consecutive rule is respected. | Off |
| 🌟 **Sheriff** | Civilian | Once per game, can choose to shoot a player at night. If the target is Mafia → target dies, Sheriff survives. If the target is a Civilian → Sheriff dies instead, target survives. Ability is consumed regardless of outcome. | Off |
| ⚫ **Godfather** | Mafia | Appears as *Civilian* to the Detective's investigation. Counts as a Mafia player for all other purposes. | Off |
| 🟠 **Jester** | Neutral | Wins if they are voted out by the Civilians during the Day phase. Has no night action. | Off |

---

## Win Conditions

| Faction | Win Condition |
|---------|--------------|
| 🔴 Mafia | The number of living Mafia ≥ the number of living Civilians |
| ⚪ Civilian | All Mafia players are eliminated |
| 🟠 Jester | Gets voted out during a Day vote (triggers immediately) |

> Win conditions are checked by the system **after every elimination**, day or night.
> If the Mafia win condition is met at the same moment a Civilian is voted out, Mafia wins.

---

## Game Flow

### 1. 🔧 Lobby & Setup Phase

1. A player creates a room and becomes the **Host**.
2. Other players join via a room code or link.
3. The Host configures the game:
   - Number of Mafia (within the allowed range for player count).
   - Which special roles to include (Detective, Doctor, Godfather, Jester).
   - Phase timer durations.
4. Once satisfied, the Host presses **Start Game**.
5. The system randomly and secretly **assigns roles** to all players.
6. Each player can view their assigned role at any time during the game via a **"My Role" button** in the UI. This button is always available and reveals the player's role privately on demand — no confirmation or forced reveal flow.
   - Mafia players also see their fellow Mafia members listed when they view their role.

---

### 2. 🌙 Night Phase

The game goes dark. A **game status card** is visible to all players, showing the current **night number** and tracking which roles have completed their night action.

#### Night Actions UI

- A **"Take Action" button** appears in the UI, **visible only to players with an active night role** (Mafia, Detective, Doctor, and Sheriff if ability not yet used).
- Pure Civilians and the Jester see a waiting screen with no action available.
- When a player completes their night action, the game status card updates to reflect their role as ready:

```
  🔴 Mafia         → Waiting...   (updates to: Mafia is ready ✅)
  🌟 Sheriff       → Waiting...   (updates to: Sheriff is ready ✅)
  🔵 Detective     → Waiting...   (updates to: Detective is ready ✅)
  🟢 Doctor        → Waiting...   (updates to: Doctor is ready ✅)
```

> Status labels show the role name only — no player identity is exposed.

---

#### 🔴 Mafia Night Action

1. A Mafia player clicks **"Take Action"**.
2. A dialogue opens listing all **eligible targets** (alive non-Mafia players, excluding the player saved by the Doctor the previous night).
3. The Mafia player selects a target and submits.
4. If there are multiple Mafia players, each submits independently. Other Mafia members see a **mark indicator** on whichever player has already been selected by a teammate — updated in real time as selections come in.
5. **Target resolution (after all Mafia have submitted or timer expires):**

| Scenario | Resolution |
|----------|------------|
| All Mafia select the same target | That player is eliminated. |
| 1–2 Mafia disagree | A random player from those selected is eliminated. |
| 3+ Mafia disagree | Majority vote wins. If no majority, a random player from the majority-tied options is eliminated. |

**Mafia targeting restrictions:**
- Mafia **cannot** target each other.
- Mafia **cannot** target the player who was saved by the Doctor the previous night.

---

#### 🌟 Sheriff Night Action

The Sheriff has a **one-time ability** usable on any night of their choosing.

1. The Sheriff clicks **"Take Action"**.
2. A dialogue opens with two options:
   - **Use Ability** — lists all alive players (excluding themselves) to shoot.
   - **Skip** — passes without using the ability (button remains available next night).
3. If the Sheriff chooses to shoot:
   - **Target is Mafia** → target is eliminated. Sheriff survives. Ability is consumed.
   - **Target is a Civilian** → Sheriff dies instead. Target survives. Ability is consumed.
4. Once the ability is used (success or fail), the Sheriff loses their "Take Action" button for the rest of the game and becomes a regular Civilian.

**Sheriff restrictions:**
- Cannot target themselves.
- The Sheriff's shot is resolved **after** the Mafia kill and **before** Doctor protection in the night order — meaning the Doctor cannot save a Sheriff's target, and the Sheriff cannot be saved by the Doctor if they misfire.

> The Sheriff's action result (who shot whom, and outcome) is **revealed publicly** at the start of the following Day Phase alongside the night's other eliminations.

---

1. The Detective clicks **"Take Action"**.
2. A dialogue opens listing all alive players (excluding themselves).
3. The Detective selects one player to investigate and submits.
4. After night resolution, the Detective privately receives the result: **Mafia** or **Civilian**.
   - If the Godfather is investigated, the result returns **Civilian**.

---

#### 🟢 Doctor Night Action

1. The Doctor clicks **"Take Action"**.
2. A dialogue opens listing all alive players.
3. The Doctor selects one player to protect and submits.

**Doctor restrictions:**
- The Doctor **cannot** protect the same player two nights in a row.
- This includes themselves — they cannot self-protect on consecutive nights.
- The Doctor **can** protect themselves multiple times across the game, as long as it's not on back-to-back nights.
- If the Doctor attempts an invalid selection (same player as last night), the system rejects it and prompts them to pick again.

---

#### Night Phase End Conditions

The Night Phase ends when **any** of the following occur:
- All active role players have submitted their actions.
- The night timer expires.
- The Host skips the phase (or skips a specific role's action window).

---

#### Night Resolution (System-Handled)

Resolved server-side in this order:
1. Doctor protection is applied.
2. Mafia's kill target is resolved against Doctor's protection:
   - **Target is protected** → no elimination. Announce: *"The night passes quietly. No one was eliminated."*
   - **Target is not protected** → player is eliminated. Role revealed (configurable).
3. Detective receives their investigation result privately.
4. Win condition is checked.

---

### 3. 🌅 Day Phase

All living players are active.

#### Stage 1 — Announcement
- The system displays who was eliminated during the night (if anyone) and their role *(if reveal-on-death is enabled)*.
- Eliminated players remain in the game as **spectators** — they can watch all public game events but cannot interact with any game mechanic.
- If a dead player attempts to interact (vote, take action, etc.), they receive a **"You are dead" dialogue** reminding them they are no longer an active participant.

#### Stage 2 — Discussion
- All living players discuss freely.
- A **discussion timer** runs (configurable, generous by default).
- The **Host can end discussion early** to move to the vote at any time.

#### Stage 3 — Vote

1. Discussion ends (timer expires or Host starts vote).
2. A **voting dialogue** opens for all living players, listing all other alive players as options.
3. Each player votes for whoever they want to eliminate, then closes the dialogue.
4. **The vote ends when:**
   - All players have voted, **OR**
   - The vote timer expires, **OR**
   - The Host skips / ends the vote early.
   - All three are treated identically — only votes already cast are counted. Players who had not yet voted are treated as abstentions and **ignored** (not counted as Spare or Eliminate).

5. A **results dialogue** is shown to all players:
   - Votes are **anonymous** — only the players who received votes and their vote counts are shown. Individual voters are not revealed.
   - Example: *"Player A — 4 votes | Player B — 2 votes | Player C — 1 vote"*

6. **Vote outcome:**

| Result | Outcome |
|--------|---------|
| One player has the most votes | That player is eliminated. Role revealed. Win condition checked. |
| Tie between two or more players | **Revote** — a new voting dialogue opens with only the tied players as options. |
| Tie again in the revote | No elimination. Night Phase begins. |

> **Jester special case:** If the Jester receives the most votes and is eliminated, the system immediately triggers their personal win. The game continues for the remaining factions.

---

### 4. 🔁 Cycle

```
[Night Phase]
  └── Role actions submitted (Mafia / Detective / Doctor)
        ↓
[Night Resolution] → Announce result → Win check
        ↓
[Day Phase]
  └── Announcement → Discussion → Vote → Results
        ↓
[Win Check] ──── Win? ──→ [End Screen]
        │
        └── No winner → back to [Night Phase]
```

---

### 5. 🏆 End Game

Triggered when a win condition is met:
1. **Winning faction** is announced with a result banner.
2. **All roles** are revealed to all players.
3. A **game summary** is shown:
   - Timeline of all eliminations (night kills and day votes in order).
   - Full role list: who was Mafia, Detective, Doctor, Godfather, Jester, Civilian.
   - Detective's full investigation log.
   - Voting history per day (vote counts per accused, anonymized).

---

## Host Controls

The Host is a **living player with a faction role** and additional game management privileges. They cannot see other players' roles or night actions — their powers are purely about **pacing**.

| Control | Available During |
|--------|----------------|
| Start Game | Lobby |
| ends Night Phase before timer ends | Night Phase |
| End Discussion early | Day — Discussion stage |
| End Vote early | Day — Vote stage (force-closes voting dialogue for all) |

> The Host retains their game management controls **even if they are eliminated**. Being dead does not remove host privileges — they can still control game pace as a spectator. Host privileges only transfer to another player if the Host explicitly **leaves the game**.

---

## Edge Cases & Rules Clarifications

| Scenario | Ruling |
|----------|--------|
| Mafia targets a Doctor-protected player | No kill. Announce quiet night. |
| Doctor tries to protect same player as previous night | System rejects the selection and prompts again. |
| Doctor self-protects on consecutive nights | Not allowed. System rejects and prompts again. |
| Doctor self-protects non-consecutively | Allowed, no limit on total self-protections. |
| Sheriff shoots a Mafia player | Mafia player is eliminated. Sheriff survives. Ability consumed. |
| Sheriff shoots a Civilian | Sheriff dies instead. Target survives. Ability consumed. |
| Sheriff shoots the Godfather | Godfather is Mafia — Sheriff survives, Godfather is eliminated. |
| Sheriff skips their action | Ability preserved for next night. Button remains available. |
| Sheriff is night-killed by Mafia before using ability | Ability is lost with them. |
| Godfather investigated by Detective | Returns *Civilian*. |
| Detective is eliminated | Their knowledge is lost. No special reveal. |
| Mafia try to target each other | Not selectable — excluded from the target dialogue. |
| Mafia try to target last night's Doctor-saved player | Not selectable — excluded from the target dialogue. |
| Jester is voted out during Day | Jester wins immediately. Game Ends. |
| Jester is night-killed by Mafia | Jester loses. Revealed as a normal elimination. |
| Living Mafia count ≥ living Civilians | Mafia wins immediately. |
| First vote ends in a tie | Revote with only tied players as options. |
| Revote also ends in a tie | No elimination. Day ends. |
| Host skips vote mid-voting | Treated identically to timer expiry. Dialogue closes for all. Only cast votes counted; abstentions ignored. |
| Player disconnects during Night | Their action is skipped after the timer ends or the host ends the night. |

---

## Configurable Settings (Set by Host in Lobby)

| Setting | Options | Default |
|---------|---------|---------|
| Number of Mafia | 1 to max for player count | 1 |
| Sheriff | On / Off | Off |
| Detective | On / Off | Off |
| Doctor | On / Off | On |
| Godfather | On / Off | Off |
| Jester | On / Off | Off |
| Night phase timer | 1 min / 2 min / 3 min / Custom | 2 min |
| Discussion timer | 2 min / 5 min / 10 min / Unlimited | 10 min |
| Vote timer | 60s / 90s / 120s | 90s |
| Reveal role on death | Always / End of game only | Always |

---

## Implementation Notes (Dev Reference)

- **Night actions** are processed server-side only. Clients only receive:
  - Confirmation that their own action was submitted.
  - Real-time mark indicators on selected targets (Mafia only, within their own action dialogue).
  - Role-ready status updates on the game status card (role label only, no player name).
- The **"Take Action" button** visibility is determined server-side and sent per-player — never rely on client-side role state alone.
- **Mafia target dialogue** must exclude Mafia players and the Doctor's previous-night save target. This exclusion list is enforced server-side.
- The **Doctor's consecutive restriction** is tracked server-side per session (last protected player ID per round).
- **Vote anonymity** — the server stores voter identity internally for audit/logging but only sends vote counts (not voter names) to clients in the results payload.
- **Tie revote** — server sends a new vote event with a restricted candidate list (tied players only).
- **Host skip on vote** — treated identically to timer expiry server-side. Server emits a force-close event to all open voting dialogues, then computes results from cast votes only. Abstentions are discarded.
- When the **Host disconnects**, host privileges transfer automatically to preserve game flow.
