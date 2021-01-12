package parser

import (
	"testing"

	"github.com/pgavlin/yomlette/internal/spec"
	"github.com/pgavlin/yomlette/lexer"
)

func TestSpec(t *testing.T) {
	var skip = map[string]bool{
		"236B": true,
		"2JQS": true,
		"2XXW": true,
		"35KP": true,
		"3HFZ": true,
		"4ABK": true,
		"4EJS": true,
		"4H7K": true,
		"4HVU": true,
		"4JVG": true,
		"55WF": true,
		"57H4": true,
		"5LLU": true,
		"5T43": true,
		"5TRB": true,
		"5U3A": true,
		"5WE3": true,
		"62EZ": true,
		"6JTT": true,
		"6JWB": true,
		"6M2F": true,
		"6PBE": true,
		"6S55": true,
		"735Y": true,
		"7LBH": true,
		"7MNF": true,
		"8KB6": true,
		"8XDJ": true,
		"9C9N": true,
		"9CWY": true,
		"9HCY": true,
		"9JBA": true,
		"9KAX": true,
		"9KBC": true,
		"9MAG": true,
		"A2M4": true,
		"BD7L": true,
		"BF9H": true,
		"BS4K": true,
		"BU8L": true,
		"C2DT": true,
		"C2SP": true,
		"CFD4": true,
		"CML9": true,
		"CN3R": true,
		"CQ3W": true,
		"CT4Q": true,
		"CTN5": true,
		"CVW2": true,
		"CXX2": true,
		"D49Q": true,
		"DFF7": true,
		"DMG6": true,
		"EB22": true,
		"EHF6": true,
		"FH7J": true,
		"FRK4": true,
		"G9HC": true,
		"GDY7": true,
		"GH63": true,
		"GT5M": true,
		"HRE5": true,
		"J7PZ": true,
		"JTV5": true,
		"JY7Z": true,
		"K858": true,
		"KK5P": true,
		"KS4U": true,
		"L94M": true,
		"LE5A": true,
		"LHL4": true,
		"M5DY": true,
		"M7A3": true,
		"N4JP": true,
		"N782": true,
		"NJ66": true,
		"NKF9": true,
		"P2EQ": true,
		"PW8X": true,
		"Q4CL": true,
		"QB6E": true,
		"QLJ7": true,
		"RHX7": true,
		"RR7F": true,
		"RXY3": true,
		"RZP5": true,
		"S3PD": true,
		"S4GJ": true,
		"S98Z": true,
		"S9E8": true,
		"SBG9": true,
		"SR86": true,
		"SU5Z": true,
		"SU74": true,
		"SY6V": true,
		"T833": true,
		"TD5N": true,
		"U44R": true,
		"U99R": true,
		"UT92": true,
		"V9D5": true,
		"W4TN": true,
		"W9L4": true,
		"WZ62": true,
		"X38W": true,
		"X4QW": true,
		"X8DW": true,
		"XW4D": true,
		"ZCZ6": true,
		"ZL4Z": true,
		"ZVH3": true,
		"ZXT5": true,
	}

	tests, err := spec.LoadLatestTests()
	if err != nil {
		t.Fatalf("failed to load tests: %v", err)
	}

	for _, test := range tests {
		t.Run(test.Name+" "+test.Description, func(t *testing.T) {
			if skip[test.Name] {
				t.Skip("skipped")
			}

			_, err := Parse(lexer.Tokenize(string(test.InputYAML)), 0)
			if test.IsError && err == nil {
				t.Fatalf("expected error during parsing")
			} else if !test.IsError && err != nil {
				t.Fatalf("unexpected error during parsing: %v", err)
			}
		})
	}
}
