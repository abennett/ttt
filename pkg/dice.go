package pkg

import (
	"errors"
	"fmt"
	"math"
	"math/rand/v2"
	"regexp"
	"strconv"
	"strings"
)

var diceRegex = regexp.MustCompile(`(\d+)d(\d+)(\+\d+|\-\d+)?`)

type DiceRoll struct {
	Count     int
	DiceSides int
	Modifier  int
}

func ParseDiceRoll(diceRoll string) (DiceRoll, error) {
	// <int>d<int>[+|-<int>]
	// (\d+)d(\d+)???
	var d DiceRoll
	matches := diceRegex.FindStringSubmatch(diceRoll)
	if len(matches) < 3 {
		return d, errors.New("string does not match expression")
	}
	parsed := make([]int, 3)
	for idx, s := range matches[1:] {
		if s == "" {
			parsed[idx] = 0
			continue
		}
		v, err := strconv.Atoi(s)
		if err != nil {
			return d, err
		}
		parsed[idx] = v
	}
	return DiceRoll{
		Count:     parsed[0],
		DiceSides: parsed[1],
		Modifier:  parsed[2],
	}, nil
}

func (dr DiceRoll) String() string {
	var builder strings.Builder
	base := fmt.Sprintf("%dd%d", dr.Count, dr.DiceSides)
	builder.WriteString(base)
	if dr.Modifier > 0 {
		builder.WriteString("+" + strconv.Itoa(dr.Modifier))
	}
	if dr.Modifier < 0 {
		absolute := int(math.Abs(float64(dr.Modifier)))
		builder.WriteString("-" + strconv.Itoa(absolute))
	}
	return builder.String()
}

func (dr DiceRoll) Roll() int {
	var result int
	for x := 0; x < dr.Count; x++ {
		result += rand.IntN(dr.DiceSides) + 1
	}
	return result + dr.Modifier
}
