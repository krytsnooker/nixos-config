package game

import "testing"

func pegs(vals ...int) []Peg {
	out := make([]Peg, len(vals))
	for i, v := range vals {
		out[i] = Peg(v)
	}
	return out
}

func TestEvaluateGuessAllBlack(t *testing.T) {
	fb := EvaluateGuess(pegs(1, 2, 3, 4), pegs(1, 2, 3, 4))
	if fb.Black != 4 || fb.White != 0 {
		t.Fatalf("got %+v", fb)
	}
}

func TestEvaluateGuessAllWhite(t *testing.T) {
	fb := EvaluateGuess(pegs(1, 2, 3, 4), pegs(4, 3, 2, 1))
	if fb.Black != 0 || fb.White != 4 {
		t.Fatalf("got %+v", fb)
	}
}

func TestEvaluateGuessDuplicates(t *testing.T) {
	fb := EvaluateGuess(pegs(1, 1, 2, 3), pegs(1, 1, 1, 1))
	if fb.Black != 2 || fb.White != 0 {
		t.Fatalf("got %+v", fb)
	}
}

func TestEvaluateGuessMixed(t *testing.T) {
	fb := EvaluateGuess(pegs(1, 2, 2, 3), pegs(2, 1, 3, 3))
	if fb.Black != 1 || fb.White != 2 {
		t.Fatalf("got %+v", fb)
	}
}

func TestGuessTurnEnforcement(t *testing.T) {
	g := NewGame("g1", "maker", "Maker")
	if err := g.SetCode("maker", pegs(1, 2, 3, 4)); err != nil {
		t.Fatal(err)
	}
	g.AddBreaker("p1", "Alice")
	g.AddBreaker("p2", "Bob")

	if _, err := g.Guess("p2", pegs(1, 1, 1, 1)); err == nil {
		t.Fatal("expected turn-order error for p2 guessing first")
	}
	if _, err := g.Guess("p1", pegs(1, 1, 1, 1)); err != nil {
		t.Fatal(err)
	}
	if _, err := g.Guess("p2", pegs(1, 1, 1, 1)); err != nil {
		t.Fatal(err)
	}
}

func TestAddBreakerRejoinIsIdempotent(t *testing.T) {
	g := NewGame("g1", "maker", "Maker")
	g.AddBreaker("p1", "Alice")
	g.AddBreaker("p1", "Alice")
	if len(g.Breakers) != 1 {
		t.Fatalf("expected 1 breaker after rejoining with the same id, got %d", len(g.Breakers))
	}
}

func TestRemoveBreakerKeepsTurnOnCorrectPlayer(t *testing.T) {
	g := NewGame("g1", "maker", "Maker")
	g.SetCode("maker", pegs(1, 2, 3, 4))
	g.AddBreaker("p1", "Alice")
	g.AddBreaker("p2", "Bob")
	g.AddBreaker("p3", "Carl")

	g.Guess("p1", pegs(5, 5, 5, 5))
	g.Guess("p2", pegs(5, 5, 5, 5))

	if err := g.RemoveBreaker("p1"); err != nil {
		t.Fatal(err)
	}

	turnID, err := g.currentTurnPlayerID()
	if err != nil {
		t.Fatal(err)
	}
	if turnID != "p3" {
		t.Fatalf("expected turn to stay on p3 after p1 left, got %s", turnID)
	}
}

func TestRemoveBreakerUnknownPlayer(t *testing.T) {
	g := NewGame("g1", "maker", "Maker")
	g.AddBreaker("p1", "Alice")
	if err := g.RemoveBreaker("nope"); err == nil {
		t.Fatal("expected error removing unknown player")
	}
}

func TestGuessWinCondition(t *testing.T) {
	g := NewGame("g1", "maker", "Maker")
	g.SetCode("maker", pegs(1, 2, 3, 4))
	g.AddBreaker("p1", "Alice")

	fb, err := g.Guess("p1", pegs(1, 2, 3, 4))
	if err != nil {
		t.Fatal(err)
	}
	if fb.Black != 4 {
		t.Fatalf("expected win, got %+v", fb)
	}
	if g.Status != StatusBreakersWon {
		t.Fatalf("expected breakers_won, got %s", g.Status)
	}
}

func TestGuessMakerWinsAfterMaxRounds(t *testing.T) {
	g := NewGame("g1", "maker", "Maker")
	g.SetCode("maker", pegs(1, 2, 3, 4))
	g.AddBreaker("p1", "Alice")

	for i := 0; i < MaxRounds; i++ {
		if _, err := g.Guess("p1", pegs(5, 5, 5, 5)); err != nil {
			t.Fatal(err)
		}
	}
	if g.Status != StatusMakerWon {
		t.Fatalf("expected maker_won, got %s", g.Status)
	}
}
