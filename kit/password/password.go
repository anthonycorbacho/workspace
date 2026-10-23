package password

import (
	"crypto/rand"
	"io"
	"math/big"

	"github.com/anthonycorbacho/workspace/kit/errors"
)

// _letters is the list of letters.
const _letters = "abcdefghijklmnopqrstuvwxyzABCDEFGHIJKLMNOPQRSTUVWXYZ"

// _digits is the list of permitted digits.
const _digits = "0123456789"

// _symbols is the list of symbols.
const _symbols = "~!@#$%^&*()_+-={}[]:<>?,."

// Generate generates a password with the given length requirements.
// digits is the number of digits
// to include in the result. symbols is the number of symbols to include in
// the result.
func Generate(length int, digits, symbols int) (string, error) {
	if length < 8 {
		return "", errors.New("password length should be higher than 8")
	}

	chars := length - digits - symbols
	if chars < 0 {
		return "", errors.New("number of digits and symbols must be less than total length")
	}

	result := ""
	reader := rand.Reader

	// Characters
	for range chars {
		ch, err := randomElement(reader, _letters)
		if err != nil {
			return "", err
		}

		result, err = randomInsert(reader, result, ch)
		if err != nil {
			return "", err
		}
	}

	// Digits
	for range digits {
		d, err := randomElement(reader, _digits)
		if err != nil {
			return "", err
		}

		result, err = randomInsert(reader, result, d)
		if err != nil {
			return "", err
		}
	}

	// Symbols
	for range symbols {
		sym, err := randomElement(reader, _symbols)
		if err != nil {
			return "", err
		}

		result, err = randomInsert(reader, result, sym)
		if err != nil {
			return "", err
		}
	}

	return result, nil
}

func randomElement(reader io.Reader, s string) (string, error) {
	n, err := rand.Int(reader, big.NewInt(int64(len(s))))
	if err != nil {
		return "", err
	}
	return string(s[n.Int64()]), nil
}

func randomInsert(reader io.Reader, s, val string) (string, error) {
	if s == "" {
		return val, nil
	}

	n, err := rand.Int(reader, big.NewInt(int64(len(s)+1)))
	if err != nil {
		return "", err
	}
	i := n.Int64()
	return s[0:i] + val + s[i:], nil
}
