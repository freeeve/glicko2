package glicko2

import (
	"encoding/json"
	"math"
	"os"
	"path/filepath"
	"testing"
)

// vector is one update_one test case decoded from testdata JSON.
type vector struct {
	Description string `json:"description"`
	Params      struct {
		GlickoTau       float64 `json:"glickoTau"`
		GlickoTolerance float64 `json:"glickoTolerance"`
	} `json:"params"`
	Player    Rating    `json:"player"`
	Opponents []Rating  `json:"opponents"`
	Scores    []float64 `json:"scores"`
	Expected  Rating    `json:"expected"`
	Tolerance float64   `json:"tolerance"`
}

// loadVectors reads and decodes a testdata vector file.
func loadVectors(t *testing.T, name string) []vector {
	t.Helper()
	data, err := os.ReadFile(filepath.Join("testdata", name))
	if err != nil {
		t.Fatalf("read %s: %v", name, err)
	}
	var out []vector
	if err := json.Unmarshal(data, &out); err != nil {
		t.Fatalf("unmarshal %s: %v", name, err)
	}
	return out
}

// approxEqual reports whether |a-b| <= tolerance.
func approxEqual(a, b, tolerance float64) bool {
	return math.Abs(a-b) <= tolerance
}

// TestUpdateVectors drives Update against the shared Glicko-2 test vectors,
// including the canonical Glickman-2012 worked example and the inactive-player
// (no-opponents) decay case.
func TestUpdateVectors(t *testing.T) {
	for _, v := range loadVectors(t, "glicko2_multi.json") {
		t.Run(v.Description, func(t *testing.T) {
			tol := v.Tolerance
			if tol == 0 {
				tol = 0.01
			}
			cfg := Config{Tau: v.Params.GlickoTau, Tolerance: v.Params.GlickoTolerance}
			got := Update(v.Player, v.Opponents, v.Scores, cfg)
			if !approxEqual(got.Rating, v.Expected.Rating, tol) {
				t.Errorf("rating: got %v, want %v (tol %v)", got.Rating, v.Expected.Rating, tol)
			}
			if !approxEqual(got.RD, v.Expected.RD, tol) {
				t.Errorf("rd: got %v, want %v (tol %v)", got.RD, v.Expected.RD, tol)
			}
			if !approxEqual(got.Vol, v.Expected.Vol, tol) {
				t.Errorf("vol: got %v, want %v (tol %v)", got.Vol, v.Expected.Vol, tol)
			}
		})
	}
}

// TestDecayMatchesEmptyUpdate verifies that Update with no opponents is exactly
// the decay path.
func TestDecayMatchesEmptyUpdate(t *testing.T) {
	p := Rating{Rating: 1723, RD: 180, Vol: 0.055}
	if Update(p, nil, nil, Config{}) != Decay(p, Config{}) {
		t.Fatal("Update with no opponents must equal Decay")
	}
}

// TestUpdateLengthMismatchPanics verifies the argument-length guard.
func TestUpdateLengthMismatchPanics(t *testing.T) {
	defer func() {
		if recover() == nil {
			t.Fatal("expected panic on opponents/scores length mismatch")
		}
	}()
	Update(Default(), []Rating{Default()}, []float64{}, Config{})
}

// FuzzUpdate asserts two always-true Glicko-2 invariants across random inputs:
// (1) all outputs are finite with positive RD/Vol, and (2) rating is monotonic
// in score — winning against a given opponent never yields a lower rating than
// losing to it. (Note: RD is not guaranteed to fall after a game, since a
// surprising result inflates volatility, so that is deliberately not asserted.)
func FuzzUpdate(f *testing.F) {
	f.Add(1500.0, 200.0, 0.06, 1400.0, 30.0, 0.5)
	f.Add(1000.0, 350.0, 0.09, 2200.0, 400.0, 1.0)
	f.Fuzz(func(t *testing.T, pr, prd, pvol, orating, ord, score float64) {
		// Constrain to a sane rating domain; skip degenerate inputs.
		if !inRange(pr, 100, 3000) || !inRange(prd, 1, 500) || !inRange(pvol, 0.01, 0.3) ||
			!inRange(orating, 100, 3000) || !inRange(ord, 1, 500) {
			t.Skip()
		}
		if math.IsNaN(score) || score < 0 || score > 1 {
			t.Skip()
		}
		player := Rating{Rating: pr, RD: prd, Vol: pvol}
		opp := Rating{Rating: orating, RD: ord, Vol: 0.06}

		got := Update(player, []Rating{opp}, []float64{score}, Config{})
		requireFinitePositive(t, got, player, opp, score)

		// Rating is monotonic in score: a win never scores below a loss.
		win := Update(player, []Rating{opp}, []float64{1}, Config{})
		loss := Update(player, []Rating{opp}, []float64{0}, Config{})
		requireFinitePositive(t, win, player, opp, 1)
		requireFinitePositive(t, loss, player, opp, 0)
		if win.Rating < loss.Rating-1e-9 {
			t.Fatalf("rating not monotonic in score: win %v < loss %v (player %+v vs %+v)",
				win.Rating, loss.Rating, player, opp)
		}
	})
}

// requireFinitePositive fails the test if r has any non-finite field or a
// non-positive RD/Vol.
func requireFinitePositive(t *testing.T, r, player, opp Rating, score float64) {
	t.Helper()
	if math.IsNaN(r.Rating) || math.IsInf(r.Rating, 0) ||
		math.IsNaN(r.RD) || math.IsInf(r.RD, 0) ||
		math.IsNaN(r.Vol) || math.IsInf(r.Vol, 0) {
		t.Fatalf("non-finite output: %+v (in %+v vs %+v score %v)", r, player, opp, score)
	}
	if r.RD <= 0 || r.Vol <= 0 {
		t.Fatalf("non-positive RD/Vol: %+v", r)
	}
}

func inRange(x, lo, hi float64) bool {
	return !math.IsNaN(x) && x >= lo && x <= hi
}
