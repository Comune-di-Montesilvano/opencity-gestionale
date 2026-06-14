package graduatoria

import "strings"

func Filtra(righe []RigaGraduatoria, fn func(RigaGraduatoria) bool) []RigaGraduatoria {
	var out []RigaGraduatoria
	for _, r := range righe {
		if fn(r) {
			out = append(out, r)
		}
	}
	return out
}

func FiltraEscluse(righe []RigaGraduatoria, prefisso string) []RigaGraduatoria {
	var out []RigaGraduatoria
	for _, r := range righe {
		if strings.HasPrefix(r.NoteEsclusione, prefisso) {
			out = append(out, r)
		}
	}
	return out
}
