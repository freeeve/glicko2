// Package glicko2 implements Mark Glickman's Glicko-2 rating system.
//
// The core entry point is [Update], which runs a single rating period for one
// player against a set of opponents and outcome scores, returning the player's
// new rating, rating deviation (RD), and volatility. [Decay] applies an
// inactive period (no games), in which the rating is unchanged and RD grows.
//
// Ratings are on the familiar Glicko scale (default 1500 / 350 / 0.06); the
// conversion to and from the internal Glicko-2 scale is handled for you.
package glicko2

import "math"

// glicko2Scale converts between the Glicko and Glicko-2 rating scales.
const glicko2Scale = 173.7178

// Solver defaults and safety rails. A non-positive or non-finite tau or
// tolerance would make the regula-falsi volatility solver diverge or never
// terminate, so [Config.resolve] clamps to these paper defaults. The iteration
// cap converts any remaining pathological numeric case into "keep the old
// volatility" instead of hanging the caller — Glickman's examples converge in
// well under 20 iterations.
const (
	defaultTau         = 0.5
	defaultTolerance   = 1e-6
	maxVolatilityIters = 100
)

// Rating is a player's Glicko-2 state on the Glicko scale: Rating is the point
// estimate, RD the rating deviation (uncertainty), and Vol the volatility.
type Rating struct {
	Rating float64 `json:"rating"`
	RD     float64 `json:"rd"`
	Vol    float64 `json:"vol"`
}

// Default returns the conventional starting rating: 1500 / 350 / 0.06.
func Default() Rating {
	return Rating{Rating: 1500, RD: 350, Vol: 0.06}
}

// Config tunes the volatility solver. The zero value is valid and uses the
// Glicko-2 paper defaults (Tau 0.5, Tolerance 1e-6). Tau constrains how much
// volatility may change between periods; smaller values dampen swings.
type Config struct {
	Tau       float64
	Tolerance float64
}

// resolve returns the effective tau/tolerance, substituting the paper defaults
// for any non-positive or non-finite value.
func (c Config) resolve() (tau, tolerance float64) {
	tau = c.Tau
	if !(tau > 0) || math.IsInf(tau, 0) {
		tau = defaultTau
	}
	tolerance = c.Tolerance
	if !(tolerance > 0) || math.IsInf(tolerance, 0) {
		tolerance = defaultTolerance
	}
	return tau, tolerance
}

// toGlicko2 converts a (rating, RD) pair into Glicko-2 (mu, phi).
func toGlicko2(rating, rd float64) (mu, phi float64) {
	return (rating - 1500.0) / glicko2Scale, rd / glicko2Scale
}

// fromGlicko2 converts (mu, phi) back to (rating, RD).
func fromGlicko2(mu, phi float64) (rating, rd float64) {
	return glicko2Scale*mu + 1500.0, glicko2Scale * phi
}

// g is the RD-attenuation function from the paper.
func g(phi float64) float64 {
	return 1.0 / math.Sqrt(1.0+3.0*phi*phi/(math.Pi*math.Pi))
}

// e is the expected score of the player against an opponent.
func e(mu, muJ, phiJ float64) float64 {
	return 1.0 / (1.0 + math.Exp(-g(phiJ)*(mu-muJ)))
}

// Update runs one Glicko-2 rating period for a single player against the given
// opponents and scores, returning the player's new rating. Each score is the
// player's result against the opponent at the same index and must lie in
// [0, 1] (1 = win, 0.5 = draw, 0 = loss); fractional scores are supported.
//
// opponents and scores must have equal length; a mismatch panics. With no
// opponents, Update is equivalent to [Decay].
func Update(player Rating, opponents []Rating, scores []float64, cfg Config) Rating {
	if len(opponents) != len(scores) {
		panic("glicko2: opponents and scores length mismatch")
	}
	if len(opponents) == 0 {
		return Decay(player, cfg)
	}

	mu, phi := toGlicko2(player.Rating, player.RD)

	muJ := make([]float64, len(opponents))
	phiJ := make([]float64, len(opponents))
	for j, op := range opponents {
		muJ[j], phiJ[j] = toGlicko2(op.Rating, op.RD)
	}

	var vInv float64
	for j := range opponents {
		gj := g(phiJ[j])
		ej := e(mu, muJ[j], phiJ[j])
		vInv += gj * gj * ej * (1.0 - ej)
	}
	v := 1.0 / vInv

	var deltaInner float64
	for j := range opponents {
		gj := g(phiJ[j])
		ej := e(mu, muJ[j], phiJ[j])
		deltaInner += gj * (scores[j] - ej)
	}
	delta := v * deltaInner

	tau, tolerance := cfg.resolve()
	newVol := solveVolatility(phi, v, delta, player.Vol, tau, tolerance)

	phiStar := math.Sqrt(phi*phi + newVol*newVol)
	phiPrime := 1.0 / math.Sqrt(1.0/(phiStar*phiStar)+1.0/v)
	muPrime := mu + phiPrime*phiPrime*deltaInner

	rating, rd := fromGlicko2(muPrime, phiPrime)
	return Rating{Rating: rating, RD: rd, Vol: newVol}
}

// Decay applies an inactive rating period in which the player faced no
// opponents: the rating is unchanged and RD grows by the volatility. cfg is
// accepted for signature symmetry with [Update] and is unused.
func Decay(player Rating, cfg Config) Rating {
	_ = cfg
	mu, phi := toGlicko2(player.Rating, player.RD)
	phiStar := math.Sqrt(phi*phi + player.Vol*player.Vol)
	rating, rd := fromGlicko2(mu, phiStar)
	return Rating{Rating: rating, RD: rd, Vol: player.Vol}
}

// solveVolatility implements the Illinois-variant regula falsi from step 5 of
// the Glicko-2 paper.
func solveVolatility(phi, v, delta, vol, tau, tolerance float64) float64 {
	a := math.Log(vol * vol)
	f := func(x float64) float64 {
		ex := math.Exp(x)
		numer := ex * (delta*delta - phi*phi - v - ex)
		denom := 2.0 * (phi*phi + v + ex) * (phi*phi + v + ex)
		return numer/denom - (x-a)/(tau*tau)
	}

	A := a
	var B float64
	if delta*delta > phi*phi+v {
		B = math.Log(delta*delta - phi*phi - v)
	} else {
		k := 1.0
		for f(a-k*tau) < 0 {
			k++
			if k > maxVolatilityIters {
				return vol
			}
		}
		B = a - k*tau
	}

	fA := f(A)
	fB := f(B)

	for i := 0; math.Abs(B-A) > tolerance; i++ {
		if i >= maxVolatilityIters {
			return vol
		}
		C := A + (A-B)*fA/(fB-fA)
		fC := f(C)
		if fC*fB <= 0 {
			A = B
			fA = fB
		} else {
			fA = fA / 2.0
		}
		B = C
		fB = fC
	}

	return math.Exp(A / 2.0)
}
