// Package cf fornisce helper per l'analisi del codice fiscale italiano.
// Formato: AAABBB80A01H501Z (16 caratteri)
//   pos 0-2:  consonanti cognome
//   pos 3-5:  consonanti nome
//   pos 6-7:  anno nascita (YY)
//   pos 8:    mese nascita (A-T, vedi meseCF)
//   pos 9-10: giorno nascita (01-31 maschio, 41-71 femmina)
//   pos 11-14: codice catastale comune nascita
//   pos 15:   carattere di controllo
package cf

import (
	"strings"
	"time"
)

var meseCF = map[byte]int{
	'A': 1, 'B': 2, 'C': 3, 'D': 4, 'E': 5, 'H': 6,
	'L': 7, 'M': 8, 'P': 9, 'R': 10, 'S': 11, 'T': 12,
}

// dispariValues è la tabella per le posizioni dispari (1,3,5,...) nel calcolo del checksum.
var dispariValues = map[byte]int{
	'A': 1, 'B': 0, 'C': 5, 'D': 7, 'E': 9, 'F': 13, 'G': 15, 'H': 17,
	'I': 19, 'J': 21, 'K': 2, 'L': 4, 'M': 18, 'N': 20, 'O': 11, 'P': 3,
	'Q': 6, 'R': 8, 'S': 12, 'T': 14, 'U': 16, 'V': 10, 'W': 22, 'X': 25,
	'Y': 24, 'Z': 23,
	'0': 1, '1': 0, '2': 5, '3': 7, '4': 9, '5': 13,
	'6': 15, '7': 17, '8': 19, '9': 21,
}

// dataNascita estrae anno, mese e giorno di nascita dal codice fiscale.
// Ritorna zero-value se il CF è malformato.
func dataNascita(cf string) (anno, mese, giorno int, ok bool) {
	cf = strings.ToUpper(strings.TrimSpace(cf))
	if len(cf) != 16 {
		return
	}
	// Anno
	if cf[6] < '0' || cf[6] > '9' || cf[7] < '0' || cf[7] > '9' {
		return
	}
	yy := int(cf[6]-'0')*10 + int(cf[7]-'0')
	currentYY := time.Now().Year() % 100
	if yy <= currentYY {
		anno = 2000 + yy
	} else {
		anno = 1900 + yy
	}

	// Mese
	m, mOk := meseCF[cf[8]]
	if !mOk {
		return
	}
	mese = m

	// Giorno
	d := int(cf[9]-'0')*10 + int(cf[10]-'0')
	if cf[9] < '0' || cf[9] > '9' || cf[10] < '0' || cf[10] > '9' {
		return
	}
	if d >= 41 {
		giorno = d - 40
	} else {
		giorno = d
	}

	ok = true
	return
}

// AnnoBirth restituisce l'anno di nascita (es. 1980). Ritorna 0 se CF non valido.
func AnnoBirth(cf string) int {
	anno, _, _, ok := dataNascita(cf)
	if !ok {
		return 0
	}
	return anno
}

// EtaAnni calcola l'età in anni interi a oggi. Ritorna -1 se CF non valido.
func EtaAnni(cf string) int {
	anno, mese, giorno, ok := dataNascita(cf)
	if !ok {
		return -1
	}
	oggi := time.Now()
	eta := oggi.Year() - anno
	if oggi.Month() < time.Month(mese) ||
		(oggi.Month() == time.Month(mese) && oggi.Day() < giorno) {
		eta--
	}
	return eta
}

// Sesso restituisce "M" o "F". Ritorna "" se CF non valido.
func Sesso(cf string) string {
	cf = strings.ToUpper(strings.TrimSpace(cf))
	if len(cf) != 16 {
		return ""
	}
	if cf[9] < '0' || cf[9] > '9' || cf[10] < '0' || cf[10] > '9' {
		return ""
	}
	d := int(cf[9]-'0')*10 + int(cf[10]-'0')
	if d >= 41 {
		return "F"
	}
	return "M"
}

// ComuneNascita restituisce il codice catastale del comune di nascita (es. "H501").
// Ritorna "" se CF non valido.
func ComuneNascita(cf string) string {
	cf = strings.ToUpper(strings.TrimSpace(cf))
	if len(cf) != 16 {
		return ""
	}
	return cf[11:15]
}

// Valido verifica il checksum del codice fiscale secondo l'algoritmo dell'Agenzia delle Entrate.
func Valido(cf string) bool {
	cf = strings.ToUpper(strings.TrimSpace(cf))
	if len(cf) != 16 {
		return false
	}
	sum := 0
	for i := 0; i < 15; i++ {
		c := cf[i]
		if i%2 == 0 {
			// posizione dispari (1-indexed): usa tabella speciale
			v, ok := dispariValues[c]
			if !ok {
				return false
			}
			sum += v
		} else {
			// posizione pari (1-indexed): valore diretto
			if c >= 'A' && c <= 'Z' {
				sum += int(c - 'A')
			} else if c >= '0' && c <= '9' {
				sum += int(c - '0')
			} else {
				return false
			}
		}
	}
	atteso := byte('A' + sum%26)
	return cf[15] == atteso
}
