package cmd

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"

	"github.com/chazu/pith"
	"github.com/spf13/cobra"

	"pudl/internal/config"
	"pudl/internal/database"
	"pudl/internal/inference"
	"pudl/internal/pithdriver"
	"pudl/internal/schema"
)

var (
	execFile      string
	execTrace     bool
	execContextKV []string
)

var execCmd = &cobra.Command{
	Use:   "exec [program]",
	Short: "Run a pith VM program against the pudl data lake",
	Long: `Execute a pith concatenative program with pudl driver words registered.

The program is a JSON array of operations. Driver words available:
  catalog/*   Catalog query and manipulation
  fact/*      Fact store operations
  schema/*    Schema operations

Programs can be provided as a JSON string argument, loaded from a file,
or piped via stdin (use - or just pipe). Stdin/file avoids shell quoting issues.

Context values (--context key=value) are available as field refs (e.g. "ctx.key").
Values are parsed as JSON when possible, otherwise treated as strings.

Examples:
    pudl exec -f program.json
    echo '["schema/list", "keys"]' | pudl exec -
    pudl exec --trace -f program.json
    pudl exec --context name=api -f query.json
    pudl exec --json '[{}, "catalog/count"]'`,
	RunE: func(cmd *cobra.Command, args []string) error {
		// Read program source
		var programJSON []byte
		var err error

		if execFile != "" {
			programJSON, err = os.ReadFile(execFile)
			if err != nil {
				return fmt.Errorf("failed to read program file: %w", err)
			}
		} else if len(args) > 0 && args[0] != "-" {
			programJSON = []byte(args[0])
		} else {
			// Read from stdin (piped or heredoc)
			programJSON, err = io.ReadAll(os.Stdin)
			if err != nil {
				return fmt.Errorf("failed to read stdin: %w", err)
			}
			if len(programJSON) == 0 {
				return fmt.Errorf("provide a program as argument, -f <file>, or pipe to stdin")
			}
		}

		// Parse JSON program
		var program []any
		if err := json.Unmarshal(programJSON, &program); err != nil {
			return fmt.Errorf("invalid program JSON: %w", err)
		}

		// Create VM
		var vm *pith.VM
		if execTrace {
			vm = pith.NewWithTrace(context.Background(), os.Stderr)
		} else {
			vm = pith.New(context.Background())
		}

		// Open database and register drivers
		configDir := config.GetPudlDir()
		db, err := database.NewCatalogDB(configDir)
		if err != nil {
			return fmt.Errorf("failed to open catalog: %w", err)
		}
		defer db.Close()

		// Build schema manager from workspace paths
		var mgr *schema.Manager
		if wsCtx != nil && len(wsCtx.SchemaSearchPaths) > 0 {
			mgr = schema.NewManagerWithPaths(wsCtx.SchemaSearchPaths...)
		} else {
			mgr = schema.NewManager(filepath.Join(configDir, "schema"))
		}

		// Create schema inferrer (best-effort — nil if schemas can't load)
		var inferrer *inference.SchemaInferrer
		if mgr != nil {
			var schemaPaths []string
			if wsCtx != nil && len(wsCtx.SchemaSearchPaths) > 0 {
				schemaPaths = wsCtx.SchemaSearchPaths
			} else {
				schemaPaths = []string{filepath.Join(configDir, "schema")}
			}
			if inf, err := inference.NewSchemaInferrer(schemaPaths...); err == nil {
				inferrer = inf
			}
		}

		pithdriver.Register(vm, db, mgr, inferrer)

		// Set context values from --context flags
		if len(execContextKV) > 0 {
			ctxMap := make(map[string]any)
			for _, kv := range execContextKV {
				parts := strings.SplitN(kv, "=", 2)
				if len(parts) != 2 {
					return fmt.Errorf("invalid --context value %q: expected key=value", kv)
				}
				ctxMap[parts[0]] = parseJSONValue(parts[1])
			}
			vm.SetContext("ctx", ctxMap)
		}

		// Run program
		if err := vm.Run(program); err != nil {
			return fmt.Errorf("execution error: %w", err)
		}

		// Print result (TOS)
		if vm.Depth() == 0 {
			return nil
		}

		result, _ := vm.Result()
		if jsonOutput {
			out, err := json.MarshalIndent(result, "", "  ")
			if err != nil {
				return fmt.Errorf("failed to marshal result: %w", err)
			}
			fmt.Println(string(out))
		} else {
			out, err := json.Marshal(result)
			if err != nil {
				return fmt.Errorf("failed to marshal result: %w", err)
			}
			fmt.Println(string(out))
		}

		return nil
	},
}

// parseJSONValue tries to parse a string as JSON; falls back to raw string.
func parseJSONValue(s string) any {
	var v any
	if err := json.Unmarshal([]byte(s), &v); err == nil {
		return v
	}
	return s
}

func init() {
	rootCmd.AddCommand(execCmd)

	execCmd.Flags().StringVarP(&execFile, "file", "f", "", "Load program from a JSON file")
	execCmd.Flags().BoolVar(&execTrace, "trace", false, "Enable trace mode (print stack after each op to stderr)")
	execCmd.Flags().StringArrayVar(&execContextKV, "context", nil, "Set context values as key=value (repeatable, values parsed as JSON)")

	execCmd.RegisterFlagCompletionFunc("file", func(cmd *cobra.Command, args []string, toComplete string) ([]string, cobra.ShellCompDirective) {
		return []string{"json"}, cobra.ShellCompDirectiveFilterFileExt
	})
}
