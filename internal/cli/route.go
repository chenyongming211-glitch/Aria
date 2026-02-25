package cli

import (
	"encoding/json"
	"fmt"
	"os"
	"text/tabwriter"

	"github.com/spf13/cobra"

	"aria/pkg/rpc"
)

var routeCmd = &cobra.Command{
	Use:   "route",
	Short: "Show Aria routing table",
	Long: `Display the routing table for the Aria VPN interface.

This command filters and displays only routes related to the Aria tunnel,
making it easier to troubleshoot routing issues compared to 'ip route show'.

Examples:
  aria route             # Show Aria routes
  aria route --json      # Output in JSON format`,
	RunE: runRoute,
}

var routeJSON bool

func init() {
	rootCmd.AddCommand(routeCmd)
	routeCmd.Flags().BoolVar(&routeJSON, "json", false, "Output in JSON format")
}

func runRoute(cmd *cobra.Command, args []string) error {
	// Check if running as root
	if os.Geteuid() != 0 {
		return fmt.Errorf("this command must be run as root")
	}

	// Create RPC client
	client := rpc.NewClient("")

	// Check if daemon is running
	if !client.IsServerRunning() {
		return fmt.Errorf("aria daemon is not running. Start it with: sudo systemctl start aria")
	}

	// Get routes
	resp, err := client.GetRoutes()
	if err != nil {
		return fmt.Errorf("failed to get routes: %w", err)
	}

	// Output in JSON if requested
	if routeJSON {
		encoder := json.NewEncoder(os.Stdout)
		encoder.SetIndent("", "  ")
		return encoder.Encode(resp)
	}

	// Format and print table
	printRoutesTable(resp)

	return nil
}

func printRoutesTable(resp *rpc.GetRoutesResponse) {
	if len(resp.Routes) == 0 {
		fmt.Println("No routes configured")
		return
	}

	// Create tabwriter for aligned columns
	// minwidth=0, tabwidth=4, padding=3 for better spacing
	w := tabwriter.NewWriter(os.Stdout, 0, 4, 3, ' ', 0)

	// Print header
	fmt.Fprintln(w, "TARGET\tTYPE\tHOSTNAME\tREGION\tSTATUS")

	// Print separator matching columns
	fmt.Fprintln(w, "────────────────────\t────────────\t────────────────\t────────────\t────────")

	// Group routes by type
	var localRoutes []rpc.RouteInfo
	var peerRoutes []rpc.RouteInfo
	var advertisedRoutes []rpc.RouteInfo

	for _, route := range resp.Routes {
		switch route.Type {
		case "local":
			localRoutes = append(localRoutes, route)
		case "peer":
			peerRoutes = append(peerRoutes, route)
		case "advertised":
			advertisedRoutes = append(advertisedRoutes, route)
		}
	}

	// Print local routes
	for _, route := range localRoutes {
		fmt.Fprintf(w, "%s\t%s\t%s\t%s\t%s\n",
			route.Target,
			route.Type,
			route.Hostname,
			route.Region,
			route.Status,
		)
	}

	// Print peer routes
	for _, route := range peerRoutes {
		fmt.Fprintf(w, "%s\t%s\t%s\t%s\t%s\n",
			route.Target,
			route.Type,
			route.Hostname,
			route.Region,
			route.Status,
		)
	}

	// Print advertised routes
	for _, route := range advertisedRoutes {
		fmt.Fprintf(w, "%s\t%s\t%s\t%s\t%s\n",
			route.Target,
			route.Type,
			route.Hostname,
			route.Region,
			route.Status,
		)
	}

	w.Flush()

	// Print summary
	fmt.Println()
	fmt.Printf("Total: %d routes (%d local, %d peer, %d advertised)\n",
		len(resp.Routes),
		len(localRoutes),
		len(peerRoutes),
		len(advertisedRoutes),
	)
}
