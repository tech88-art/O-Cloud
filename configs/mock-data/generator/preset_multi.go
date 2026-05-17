// preset_multi is the set-b-multi-site placeholder.
//
// Implementation is deferred to P1-T-307. The generator returns an error
// so `make gen-multi` fails loudly rather than producing an empty dataset
// that would slip past schema validation.
package main

import (
	"errors"

	"github.com/example/ocloud-edge/configs/mock-data/generator/pkg/model"
)

var errMultiNotImplemented = errors.New("preset multi not implemented yet (see P1-T-307)")

func buildSetBMultiSite() (*model.Dataset, error) {
	return nil, errMultiNotImplemented
}
