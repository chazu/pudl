package inference

import (
	"fmt"
	"io"
)

// WarnLoadErrors writes one summary line to w when any schema package under
// schemaPaths failed to load or a base_schema cycle exists, and reports whether
// it wrote one.
//
// A broken schema package does not stop an import: its data falls back to the
// catch-all schema. Without this line that looks exactly like "my data did not
// match", so commands that infer schemas print it to stderr and point at
// `pudl doctor`, which lists each problem.
func WarnLoadErrors(w io.Writer, schemaPaths ...string) bool {
	if len(schemaPaths) == 0 {
		return false
	}
	si, err := Shared(schemaPaths...)
	if err != nil {
		return false
	}

	loadErrors := len(si.LoadErrors())
	cycles := len(si.BaseSchemaCycles())
	if loadErrors == 0 && cycles == 0 {
		return false
	}

	var problems string
	switch {
	case loadErrors > 0 && cycles > 0:
		problems = fmt.Sprintf("%s and %s", plural(loadErrors, "schema load error"), plural(cycles, "base_schema cycle"))
	case loadErrors > 0:
		problems = plural(loadErrors, "schema load error")
	default:
		problems = plural(cycles, "base_schema cycle")
	}
	fmt.Fprintf(w, "warning: %s; affected data falls back to the catch-all schema (run 'pudl doctor' for details)\n", problems)
	return true
}

func plural(n int, noun string) string {
	if n == 1 {
		return fmt.Sprintf("1 %s", noun)
	}
	return fmt.Sprintf("%d %ss", n, noun)
}
