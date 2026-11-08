package game

import (
	"crypto/rand"
	"errors"
	"math/big"
	"time"
)

const (
	CodeLength = 4
	NumColors  = 6
	MaxRounds  = 10
)

type Status string

const (
	StatusWaitingForCode Status = "waiting_for_code"
	StatusInProgress     Status = "in_progress"
	StatusBreakersWon    Status = "breakers_won"
	StatusMakerWon       Status = "maker_won"
)

type Peg int

type Feedback struct {
	Black          int   `json:"black"`
	White          int   `json:"white"`
	ExactPositions []int `json:"exactPositions"`
}

type Guess struct {
	PlayerID string    `json:"playerId"`
	Pegs     []Peg     `json:"pegs"`
	Feedback Feedback  `json:"feedback"`
	At       time.Time `json:"at"`
}

type Player struct {
	ID   string `json:"id"`
	Name string `json:"name"`
}

type Game struct {
	ID        string
	MakerID   string
	MakerName string
	Breakers  []Player
	code      []Peg
	History   []Guess
	Status    Status
	TurnIndex int
	CreatedAt time.Time
}

func NewGame(id, makerID, makerName string) *Game {
	return &Game{
		ID:        id,
		MakerID:   makerID,
		MakerName: makerName,
		Breakers:  []Player{},
		History:   []Guess{},
		Status:    StatusWaitingForCode,
		TurnIndex: 0,
		CreatedAt: time.Now(),
	}
}

func GenerateCode() ([]Peg, error) {
	code := make([]Peg, CodeLength)
	for i := range code {
		n, err := rand.Int(rand.Reader, big.NewInt(NumColors))
		if err != nil {
			return nil, err
		}
		code[i] = Peg(n.Int64() + 1)
	}
	return code, nil
}

func ValidatePegs(pegs []Peg) error {
	if len(pegs) != CodeLength {
		return errors.New("code must have exactly 4 pegs")
	}
	for _, p := range pegs {
		if p < 1 || p > NumColors {
			return errors.New("peg value out of range")
		}
	}
	return nil
}

func (g *Game) SetCode(playerID string, pegs []Peg) error {
	if playerID != g.MakerID {
		return errors.New("only the codemaker can set the code")
	}
	if g.Status != StatusWaitingForCode {
		return errors.New("code already set")
	}
	if err := ValidatePegs(pegs); err != nil {
		return err
	}
	g.code = pegs
	g.Status = StatusInProgress
	return nil
}

func (g *Game) AddBreaker(playerID, name string) error {
	if playerID == g.MakerID {
		return errors.New("codemaker cannot also be a codebreaker")
	}
	if g.Status == StatusBreakersWon || g.Status == StatusMakerWon {
		return errors.New("game has already finished")
	}
	for _, b := range g.Breakers {
		if b.ID == playerID {
			return nil
		}
	}
	g.Breakers = append(g.Breakers, Player{ID: playerID, Name: name})
	return nil
}

func (g *Game) RemoveBreaker(playerID string) error {
	idx := -1
	for i, b := range g.Breakers {
		if b.ID == playerID {
			idx = i
			break
		}
	}
	if idx == -1 {
		return errors.New("player is not in this game")
	}

	currentID, _ := g.currentTurnPlayerID()
	g.Breakers = append(g.Breakers[:idx], g.Breakers[idx+1:]...)

	if len(g.Breakers) == 0 {
		g.TurnIndex = 0
		return nil
	}
	for i, b := range g.Breakers {
		if b.ID == currentID {
			g.TurnIndex = i
			return nil
		}
	}
	g.TurnIndex = g.TurnIndex % len(g.Breakers)
	return nil
}

func (g *Game) currentTurnPlayerID() (string, error) {
	if len(g.Breakers) == 0 {
		return "", errors.New("no codebreakers have joined")
	}
	return g.Breakers[g.TurnIndex%len(g.Breakers)].ID, nil
}

func (g *Game) Guess(playerID string, pegs []Peg) (Feedback, error) {
	if g.Status != StatusInProgress {
		return Feedback{}, errors.New("game is not in progress")
	}
	turnPlayerID, err := g.currentTurnPlayerID()
	if err != nil {
		return Feedback{}, err
	}
	if playerID != turnPlayerID {
		return Feedback{}, errors.New("it is not your turn")
	}
	if err := ValidatePegs(pegs); err != nil {
		return Feedback{}, err
	}

	fb := EvaluateGuess(g.code, pegs)
	g.History = append(g.History, Guess{
		PlayerID: playerID,
		Pegs:     pegs,
		Feedback: fb,
		At:       time.Now(),
	})

	if fb.Black == CodeLength {
		g.Status = StatusBreakersWon
	} else if len(g.History) >= MaxRounds {
		g.Status = StatusMakerWon
	} else {
		g.TurnIndex++
	}

	return fb, nil
}

func EvaluateGuess(secret, guess []Peg) Feedback {
	black := 0
	exactPositions := []int{}
	secretRemain := make([]Peg, 0, len(secret))
	guessRemain := make([]Peg, 0, len(guess))

	for i := range secret {
		if secret[i] == guess[i] {
			black++
			exactPositions = append(exactPositions, i)
		} else {
			secretRemain = append(secretRemain, secret[i])
			guessRemain = append(guessRemain, guess[i])
		}
	}

	white := 0
	used := make([]bool, len(secretRemain))
	for _, gp := range guessRemain {
		for i, sp := range secretRemain {
			if !used[i] && sp == gp {
				used[i] = true
				white++
				break
			}
		}
	}

	return Feedback{Black: black, White: white, ExactPositions: exactPositions}
}

func (g *Game) RevealCode() []Peg {
	return g.code
}
