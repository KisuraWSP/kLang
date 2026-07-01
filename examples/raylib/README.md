# Kibi Catch

An interactive native raylib game written in kLang. Catch falling orbs with the
paddle, build your score, and survive as the game speeds up.

## Controls

- `A` / `D` or left/right arrows: move the paddle.
- Hold the left mouse button: drag the paddle.
- `Enter`: restart after game over.
- `Escape`: quit.

Run it from the repository root:

```sh
go run . run examples/raylib
```

The example demonstrates window management, keyboard and mouse input, mutable
game state, random values, collision checks, drawing primitives, colors, text,
and a restartable game loop. The first build may take longer because raylib's C
sources are compiled with the Go binding.
