package pkg

import (
	"testing"
)

func TestParseDiceRoll(t *testing.T) {
	example := DiceRoll{
		Count:     1,
		DiceSides: 20,
		Modifier:  1,
	}
	dr, err := ParseDiceRoll("1d20+1")
	if err != nil {
		t.Fatal(err)
	}
	if dr != example {
		t.Fatal()
	}

	_, err = ParseDiceRoll("cantaloupe")
	if err == nil {
		t.Fatal("that definitely shouldn't work")
	}
}

func TestDiceRollString(t *testing.T) {
	dr := DiceRoll{
		Count:     1,
		DiceSides: 20,
		Modifier:  0,
	}
	if "1d20" != dr.String() {
		t.Fatalf("%s should equal 1d20", dr.String())
	}
}
