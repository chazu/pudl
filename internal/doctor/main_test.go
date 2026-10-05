package doctor

import (
	"os"
	"testing"

	"github.com/chazu/pudl/internal/testenv"
)

// TestMain keeps every test in this package away from the real ~/.pudl and ~/.mu.
func TestMain(m *testing.M) { os.Exit(testenv.RunWithIsolatedHome(m)) }
