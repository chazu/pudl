package cmd

import (
	"fmt"
	"net"
	"os/exec"
	"os/signal"
	"runtime"
	"syscall"

	"github.com/spf13/cobra"

	"github.com/chazu/pudl/internal/uiserver"
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

Schema changes (new types, _pudl.graph blocks) take effect after a restart;
new observations and rules do not need one.

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

	opts := uiserver.Options{
		Assets:    webui.Assets(),
		Graph:     buildWorkspaceGraph,
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
