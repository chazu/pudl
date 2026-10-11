package cmd

import (
	"encoding/json"
	"fmt"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestCommandInventoryMatchesCompositeIdentityWithoutRoutingTag(t *testing.T) {
	pudlDir := cliWorkspace(t)
	writeFile(t, filepath.Join(pudlDir, "schema", "pudl", "gcp", "firewall.cue"), fmt.Sprintf(e2eFirewallBase, e2eFirewallFacts))
	input := filepath.Join(t.TempDir(), "records.json")
	writeFile(t, input, `[{"kind":"compute#firewall","project":"a","name":"same","disabled":false},{"kind":"compute#firewall","project":"b","name":"same","disabled":true}]`)
	writeFile(t, filepath.Join(pudlDir, "schema", "models", "identity.cue"), fmt.Sprintf(`package models
import sm "pudl.schemas/pudl/systemmodel@v0"
#Identity: sm.#SystemModel & {
 name:"identity"
 observation:{scope:"projects/firewalls",complete:true}
 populate:{schema:"pudl/gcp.#Firewall",runs:[{argv:["cat",%q]}]}
 desired:[{"_schema":"gcp.firewall",project:"a",name:"same",disabled:false}]
}`, input))
	r := runCLI(t, "run", "identity", "--json", "--detailed-exitcode")
	require.NoError(t, r.Err, r.Stdout+r.Stderr)
	var report RunReport
	require.NoError(t, json.Unmarshal([]byte(r.Stdout), &report))
	require.True(t, report.Drift.Clean, r.Stdout)
	require.True(t, report.Drift.Verified)
}
