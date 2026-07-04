# glicko2

[![Go Reference](https://pkg.go.dev/badge/github.com/freeeve/glicko2.svg)](https://pkg.go.dev/github.com/freeeve/glicko2)

A small, dependency-free Go implementation of [Mark Glickman's Glicko-2 rating
system](http://www.glicko.net/glicko/glicko2.pdf).

It exposes a single-player, single-rating-period update on the familiar Glicko
scale (1500 / 350 / 0.06). Fractional scores are supported, so it works for
win/loss/draw games as well as any outcome expressed as a value in `[0, 1]`.

## Install

```sh
go get github.com/freeeve/glicko2
```

## Usage

```go
package main

import (
	"fmt"

	"github.com/freeeve/glicko2"
)

func main() {
	player := glicko2.Rating{Rating: 1500, RD: 200, Vol: 0.06}
	opponents := []glicko2.Rating{
		{Rating: 1400, RD: 30, Vol: 0.06},
		{Rating: 1550, RD: 100, Vol: 0.06},
		{Rating: 1700, RD: 300, Vol: 0.06},
	}
	scores := []float64{1, 0, 0} // win, loss, loss

	// One rating period. Config{} uses the paper defaults (Tau 0.5, Tol 1e-6).
	updated := glicko2.Update(player, opponents, scores, glicko2.Config{})
	fmt.Printf("%+v\n", updated) // ~ {Rating:1464.06 RD:151.52 Vol:0.05999}

	// A period with no games: rating holds, RD grows.
	idle := glicko2.Decay(player, glicko2.Config{})
	_ = idle

	// A fresh player.
	newbie := glicko2.Default() // {1500, 350, 0.06}
	_ = newbie
}
```

## API

- `Rating{Rating, RD, Vol}` — a player's state on the Glicko scale.
- `Default() Rating` — the conventional starting rating, `{1500, 350, 0.06}`.
- `Config{Tau, Tolerance}` — solver tuning; the zero value uses paper defaults.
- `Update(player, opponents, scores, cfg) Rating` — one rating period. `scores`
  are in `[0, 1]` and must match `opponents` in length. With no opponents this
  is equivalent to `Decay`.
- `Decay(player, cfg) Rating` — an inactive period: rating unchanged, RD grows.

To run several periods, feed the previous result back into the next `Update`.
Batch multiple games in the same period into one `Update` call rather than
calling it once per game.

## Notes

The volatility solver is the Illinois-variant regula falsi from step 5 of the
paper, with safety rails: non-finite/non-positive `Tau`/`Tolerance` fall back to
the defaults, and a pathological numeric case keeps the prior volatility instead
of failing to converge.

## Credit

Extracted and generalized from a rating engine originally written for a ladder
project; verified against the canonical Glickman-2012 worked example.

## License

[MIT](./LICENSE)
