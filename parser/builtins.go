package parser

var builtins = map[string]interface{}{
	"and":      true,
	"call":     true,
	"html":     true,
	"index":    true,
	"slice":    true,
	"js":       true,
	"len":      true,
	"not":      true,
	"or":       true,
	"print":    true,
	"printf":   true,
	"println":  true,
	"urlquery": true,

	// Comparisons
	"eq": true, // ==
	"ge": true, // >=
	"gt": true, // >
	"le": true, // <=
	"lt": true, // <
	"ne": true, // !=
}
