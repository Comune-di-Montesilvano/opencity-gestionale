// Package mense implementa il motore di calcolo graduatoria per il servizio
// "Rimborso spese rette e mense scolastiche" (bando FSE+ Abruzzo 2021-2027).
package mense

import (
	"opencity-gestionale/internal/graduatoria"
	"opencity-gestionale/internal/opencity"
)

const EngineName = "mense_rette"

func init() {
	graduatoria.Register(&Engine{})
}

// Engine implementa graduatoria.ServiceEngine per rette e mense scolastiche.
type Engine struct{}

func (e *Engine) Name() string { return EngineName }

func (e *Engine) Calcola(apps []opencity.Application, cfg graduatoria.BandoConfig) (*graduatoria.Graduatoria, error) {
	return graduatoria.CalcolaConConfig(apps, cfg)
}

func (e *Engine) CSVHeaders() []string {
	return graduatoria.HeaderGraduatoria
}

func (e *Engine) CSVRecord(categoria string, r graduatoria.RigaGraduatoria) []string {
	return graduatoria.RigaToRecord(categoria, r)
}
