package scanner

import (
	"testing"

	"github.com/pgavlin/yomlette/token"
	"github.com/stretchr/testify/assert"
)

func TestSingleQuote(t *testing.T) {
	cases := map[string]string{
		`'foo'`:   `foo`,
		`'''foo'`: `'foo`,
		`'foo'''`: `foo'`,
		`'"foo"'`: `"foo"`,
		`'f''oo'`: `f'oo`,
	}
	for input, expected := range cases {
		t.Run(input, func(t *testing.T) {
			var s Scanner
			s.Init(input)
			tokens, err := s.Scan()
			if !assert.NoError(t, err) {
				return
			}
			if !assert.Len(t, tokens, 1) {
				return
			}
			assert.Equal(t, token.SingleQuoteType, tokens[0].Type)
			assert.Equal(t, expected, tokens[0].Value)
		})
	}
}
