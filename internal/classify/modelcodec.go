package classify

import (
	"bytes"
	"encoding/gob"
	"fmt"
)

// modelWire is Model with its fields exported, for gob.
type modelWire struct {
	Cats    []string
	Prior   map[string]float64
	All     float64
	Meta    map[string]map[string]float64
	MetaAll map[string]float64
	Word    map[string]map[string]float64
	WordAll map[string]float64
	WordN   map[string]int
	Blind   map[string]map[string]map[string]float64
	BlindN  map[string]map[string]int
	// Contested is new in train2; a model kept from before has none, and
	// overrules no generic rule until it is trained again.
	Contested map[string]bool
}

// MarshalBinary encodes the model, so a trained one can be kept.
func (m *Model) MarshalBinary() ([]byte, error) {
	var b bytes.Buffer
	err := gob.NewEncoder(&b).Encode(modelWire{m.cats, m.prior, m.all, m.meta, m.metaAll, m.word, m.wordAll, m.wordN, m.blind, m.blindN, m.contested})
	if err != nil {
		return nil, fmt.Errorf("classify: encode model: %w", err)
	}
	return b.Bytes(), nil
}

// UnmarshalBinary decodes a model MarshalBinary encoded.
func (m *Model) UnmarshalBinary(data []byte) error {
	var w modelWire
	if err := gob.NewDecoder(bytes.NewReader(data)).Decode(&w); err != nil {
		return fmt.Errorf("classify: decode model: %w", err)
	}
	*m = Model{w.Cats, w.Prior, w.All, w.Meta, w.MetaAll, w.Word, w.WordAll, w.WordN, w.Blind, w.BlindN, w.Contested}
	// gob leaves empty maps nil; Train never does.
	for _, p := range []*map[string]float64{&m.prior, &m.metaAll, &m.wordAll} {
		if *p == nil {
			*p = map[string]float64{}
		}
	}
	if m.meta == nil {
		m.meta = map[string]map[string]float64{}
	}
	if m.word == nil {
		m.word = map[string]map[string]float64{}
	}
	if m.wordN == nil {
		m.wordN = map[string]int{}
	}
	if m.blind == nil {
		m.blind = map[string]map[string]map[string]float64{}
	}
	if m.blindN == nil {
		m.blindN = map[string]map[string]int{}
	}
	if m.contested == nil {
		m.contested = map[string]bool{}
	}
	return nil
}
