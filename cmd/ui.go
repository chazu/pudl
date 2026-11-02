package cmd

import (
	"context"
	"fmt"
	"net"
	"os/exec"
	"os/signal"
	"runtime"
	"sync"
	"syscall"
	"time"

	"github.com/spf13/cobra"

	"github.com/chazu/pudl/internal/database"
	"github.com/chazu/pudl/internal/graphexport"
	"github.com/chazu/pudl/internal/inference"
	"github.com/chazu/pudl/internal/uiserver"
	"github.com/chazu/pudl/internal/validator"
	"github.com/chazu/pudl/internal/webui"
)

const defaultUIAddr = "127.0.0.1:7468"

var (
	uiAddr        string
	uiNoOpen      bool
	uiAllowRemote bool
)

var uiCmd = &cobra.Command{
	Use:   "ui",
	Short: "Open the graph explorer on this workspace's catalog",
	Args:  cobra.NoArgs,
	Long: `Serve the web graph explorer, embedded in the pudl binary, and open it in
a browser. The explorer reads the workspace catalog live: every load or
Reload shows the current resources and relationships (the same graph as
'pudl graph export').

The server is read-only and listens on the loopback interface. It answers
only requests addressed to that host, so other web pages cannot read the
inventory. Serving on another interface exposes the inventory to anyone who
can reach it, so it needs --allow-remote.

The UI refreshes itself when the catalog, the rules or the schemas change
(for example when a 'pudl check' finishes in another terminal), keeping the
current view.

Examples:
    pudl ui
    pudl ui --addr 127.0.0.1:9000 --no-open`,
	RunE: pudlRunE(runUI),
}

func init() {
	rootCmd.AddCommand(uiCmd)
	uiCmd.Flags().StringVar(&uiAddr, "addr", defaultUIAddr, "Address to listen on (port 0 picks a free port)")
	uiCmd.Flags().BoolVar(&uiNoOpen, "no-open", false, "Do not open a browser")
	uiCmd.Flags().BoolVar(&uiAllowRemote, "allow-remote", false, "Allow a non-loopback --addr (exposes the inventory)")
}

func runUI(cmd *cobra.Command, args []string) error {
	host, _, err := net.SplitHostPort(uiAddr)
	if err != nil {
		return fmt.Errorf("invalid --addr %q: %w", uiAddr, err)
	}
	remote := !uiserver.IsLoopback(host)
	if remote && !uiAllowRemote {
		return fmt.Errorf("--addr %s is not a loopback address; pass --allow-remote to expose the inventory on it", uiAddr)
	}
	ln, err := net.Listen("tcp", uiAddr)
	if err != nil {
		return fmt.Errorf("listen on %s: %w (another pudl ui? try --addr 127.0.0.1:0)", uiAddr, err)
	}

	src, err := newUIGraphSource()
	if err != nil {
		ln.Close()
		return err
	}
	defer src.close()
	opts := uiserver.Options{
		Assets:    webui.Assets(),
		Graph:     src.graph,
		Stamp:     src.stamp,
		Snapshots: src.snapshots,
		Workspace: effectiveWorkspaceName(),
		Version:   version,
	}
	if remote {
		// Accept only the address the user chose, as typed and as bound.
		opts.AllowedHosts = []string{uiAddr, ln.Addr().String()}
	}

	url := "http://" + ln.Addr().String() + "/"
	fmt.Fprintf(errw(), "pudl ui: serving %s at %s (Ctrl-C to stop)\n", opts.Workspace, url)
	if !uiNoOpen {
		if err := openBrowser(url); err != nil {
			fmt.Fprintf(errw(), "pudl ui: could not open a browser (%v); open %s yourself\n", err, url)
		}
	}
	ctx, stop := signal.NotifyContext(cmd.Context(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()
	return uiserver.Serve(ctx, ln, uiserver.Handler(opts))
}

// openBrowser opens url with the platform's default handler.
func openBrowser(url string) error {
	var c *exec.Cmd
	switch runtime.GOOS {
	case "darwin":
		c = exec.Command("open", url)
	case "windows":
		c = exec.Command("rundll32", "url.dll,FileProtocolHandler", url)
	default:
		c = exec.Command("xdg-open", url)
	}
	return c.Start()
}

// uiGraphSource builds graphs for pudl ui and fingerprints what they depend
// on: the catalog, the rule files and the schema files.
type uiGraphSource struct {
	db          *database.CatalogDB
	ruleDirs    []string
	schemaDirs  []string
	mu          sync.Mutex
	builtSchema string
}

func newUIGraphSource() (*uiGraphSource, error) {
	configDir := effectivePudlDir()
	db, err := database.NewCatalogDB(configDir)
	if err != nil {
		return nil, fmt.Errorf("failed to open catalog: %w", err)
	}
	cfg, err := loadEffectiveConfig()
	if err != nil {
		db.Close()
		return nil, err
	}
	return &uiGraphSource{db: db, ruleDirs: queryRulePaths(configDir), schemaDirs: effectiveSchemaPaths(cfg)}, nil
}

func (s *uiGraphSource) close() { s.db.Close() }

func (s *uiGraphSource) stamp(context.Context) (string, error) {
	catalog, err := s.db.ChangeStamp()
	if err != nil {
		return "", err
	}
	return catalog + "|" + uiserver.FileTreeStamp(s.ruleDirs...) + "|" + uiserver.FileTreeStamp(s.schemaDirs...), nil
}

// graph builds the current graph. Schemas are cached per process, so when
// the schema files changed since the last build the caches are reset first.
func (s *uiGraphSource) graph(ctx context.Context, asOf *time.Time) (*graphexport.Graph, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if schema := uiserver.FileTreeStamp(s.schemaDirs...); schema != s.builtSchema {
		if s.builtSchema != "" {
			inference.ResetShared()
			validator.ResetSharedLoaders()
		}
		s.builtSchema = schema
	}
	return buildWorkspaceGraph(ctx, asOf)
}

// snapshots lists the most recent observation snapshots for as-of picks.
func (s *uiGraphSource) snapshots(context.Context) ([]uiserver.Snapshot, error) {
	list, err := s.db.ListObserveSnapshots("", 200)
	if err != nil {
		return nil, err
	}
	out := make([]uiserver.Snapshot, len(list))
	for i, snap := range list {
		out[i] = uiserver.Snapshot{ID: snap.SnapshotID, CreatedAt: snap.CreatedAt, Model: snap.Model, Scope: snap.Scope, Complete: snap.Complete}
	}
	return out, nil
}
