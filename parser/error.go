package parser

import (
	"bytes"

	"github.com/pgavlin/yomlette/internal/errors"
	"golang.org/x/xerrors"
)

// FormatError is a utility function that takes advantage of the metadata
// stored in the errors returned by this package's parser.
//
// If the second argument `colored` is true, the error message is colorized.
// If the third argument `inclSource` is true, the error message will
// contain snippets of the YAML source that was used.
func FormatError(e error, colored, inclSource bool) string {
	var pp errors.PrettyPrinter
	if xerrors.As(e, &pp) {
		var buf bytes.Buffer
		pp.PrettyPrint(&errors.Sink{&buf}, colored, inclSource)
		return buf.String()
	}

	return e.Error()
}
