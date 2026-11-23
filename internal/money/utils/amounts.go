package utils

import (
	"fmt"
	"math"
)

// ToMinor is amount, as the Tools send it, in currency's minor unit: whole
// yen, or centavos. It refuses yen with decimals and pesos past centavos.
func ToMinor(currency string, amount float64) (int64, error) {
	if currency == "JPY" {
		if amount != math.Trunc(amount) {
			return 0, Refusal{Message: fmt.Sprintf("%v isn't whole yen", amount)}
		}
		return int64(amount), nil
	}
	centavos := math.Round(amount * 100)
	if math.Abs(centavos-amount*100) > 1e-6 {
		return 0, Refusal{Message: fmt.Sprintf("%v has more than 2 decimals: pesos go to centavos", amount)}
	}
	return int64(centavos), nil
}

// FromMinor is minor, in currency's minor unit, as the Tools answer it.
func FromMinor(currency string, minor int64) float64 {
	if currency == "JPY" {
		return float64(minor)
	}
	return float64(minor) / 100
}
