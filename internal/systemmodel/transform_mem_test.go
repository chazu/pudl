package systemmodel

import (
	"os"
	"runtime"
	"testing"

	"cuelang.org/go/cue/cuecontext"
)

func TestTransformRetainedHeap(t *testing.T) {
	if os.Getenv("PUDL_TRANSFORM_MEM") == "" {
		t.Skip("set PUDL_TRANSFORM_MEM=1")
	}
	tr := &Transform{value: cuecontext.New().CompileString(`{in: _, out: in.versionedResources[0].resource}`)}
	var ms runtime.MemStats
	for round := 1; round <= 4; round++ {
		for i := 0; i < 5000; i++ {
			if _, _, err := tr.Apply(caiInstance(i)); err != nil {
				t.Fatal(err)
			}
		}
		runtime.GC()
		runtime.ReadMemStats(&ms)
		t.Logf("after %5d records: heap in use %d MB", round*5000, ms.HeapInuse>>20)
	}
}
