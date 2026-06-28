# Command Line Arena

Runs a deterministic command-line combat match between two fighters. The
six-file project separates fighter state, damage rules, arena presentation,
simulation flow, and the application entry point.

Every third round triggers a power surge. The match tracks damage and health,
stops when a fighter reaches zero HP, and reports the winner.

## Files

- `player.klang` creates fighters and applies immutable health updates.
- `rules.klang` calculates damage and power-surge bonuses.
- `arena.klang` renders fighters, rounds, and combat events.
- `simulation.klang` runs the turn-based match.
- `app.klang` configures the fighters and verifies the result.
- `first.klang` is the project entry file.
- `klang.project` defines the runnable workspace.

## Try It

From the repository root:

```sh
go run . check examples/commandlinearena
go run . run examples/commandlinearena
```

Expected program output:

```text
COMMAND LINE ARENA
==================
RED : Nyx - HP 40 ATK 12 DEF 4
BLUE: Rook - HP 44 ATK 10 DEF 5

ROUND 1
Nyx hits Rook for 7 damage - 37 HP left
Rook hits Nyx for 6 damage - 34 HP left

ROUND 2
Nyx hits Rook for 7 damage - 30 HP left
Rook hits Nyx for 6 damage - 28 HP left

ROUND 3 - POWER SURGE
Nyx hits Rook for 12 damage - 18 HP left
Rook hits Nyx for 11 damage - 17 HP left

ROUND 4
Nyx hits Rook for 7 damage - 11 HP left
Rook hits Nyx for 6 damage - 11 HP left

ROUND 5
Nyx hits Rook for 7 damage - 4 HP left
Rook hits Nyx for 6 damage - 5 HP left

ROUND 6 - POWER SURGE
Nyx hits Rook for 12 damage - 0 HP left

MATCH COMPLETE
Winner: Nyx
Rounds: 6
Final health: 5
```
