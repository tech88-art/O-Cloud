// preset_stress is the set-c-stress placeholder.
//
// Implementation is deferred to P1-T-307. Same pattern as preset_multi:
// fail loudly so the placeholder cannot be mistaken for a real dataset.
package main

import (
	"errors"

	"github.com/example/ocloud-edge/configs/mock-data/generator/pkg/model"
)

var errStressNotImplemented = errors.New("preset stress not implemented yet (see P1-T-307)")

func buildSetCStress() (*model.Dataset, error) {
	return nil, errStressNotImplemented
}
