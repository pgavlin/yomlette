package scanner

import (
	"io"
	"testing"

	"github.com/pgavlin/yomlette/internal/spec"
)

func TestSpec(t *testing.T) {
	tests, err := spec.LoadLatestTests()
	if err != nil {
		t.Fatalf("failed to load tests: %v", err)
	}

	for _, test := range tests {
		t.Run(test.Name+" "+test.Description, func(t *testing.T) {
			var s Scanner
			s.Init(string(test.InputYAML))

			for {
				_, err := s.Scan()
				switch err {
				case io.EOF:
					return
				case nil:
					// OK
				default:
					t.Fatalf("unexpected error during scanning: %v (%v)", err, s.pos())
				}
			}
		})
	}
}
