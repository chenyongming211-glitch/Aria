package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"strings"

	"aria/internal/auth"

	"github.com/spf13/cobra"
)

var (
	controllerURL string
	authToken     string
	tenantID      string
	version       = "0.2.26-test-7" // 默认开发版本，通过 ldflags 注入
)

var rootCmd = &cobra.Command{
	Use:   "ariactl",
	Short: "Aria Controller CLI - Manage SD-WAN network",
	Long: `ariactl is the command-line tool for managing Aria SD-WAN Controller.

Use this tool to manage network routes, view nodes, and configure the network.`,
}

var networkCmd = &cobra.Command{
	Use:   "network",
	Short: "Manage network routes (Site-to-Site VPN)",
	Long: `Manage network routes advertised by nodes for Site-to-Site VPN.

This command allows you to add, remove, and list network routes for any node
in the SD-WAN network. All changes are synchronized to peers automatically.`,
}

var networkListCmd = &cobra.Command{
	Use:   "list [hostname]",
	Short: "List advertised network routes",
	Long: `List all advertised network routes.

If hostname is provided, only show routes for that specific node.
Otherwise, show routes for all nodes.

Examples:
  ariactl network list                # List all routes
  ariactl network list VM-0-2-ubuntu  # List routes for specific node`,
	RunE: runNetworkList,
}

var networkAddCmd = &cobra.Command{
	Use:   "add <hostname> <CIDR>",
	Short: "Add a network route for a node",
	Long: `Add a CIDR network to a node's advertised routes.

The route will be automatically synchronized to all peers in the network.

Examples:
  ariactl network add VM-0-2-ubuntu 172.16.0.0/24`,
	Args: cobra.ExactArgs(2),
	RunE: runNetworkAdd,
}

var networkRemoveCmd = &cobra.Command{
	Use:   "remove <hostname> <CIDR>",
	Short: "Remove a network route from a node",
	Long: `Remove a CIDR network from a node's advertised routes.

The route will be automatically removed from all peers in the network.

Examples:
  ariactl network remove VM-0-2-ubuntu 172.16.0.0/24`,
	Args: cobra.ExactArgs(2),
	RunE: runNetworkRemove,
}

func init() {
	rootCmd.PersistentFlags().StringVar(&controllerURL, "controller", "http://localhost:8080", "Controller URL")
	rootCmd.PersistentFlags().StringVar(&authToken, "token", os.Getenv("ARIACTL_TOKEN"), "Controller JWT token; defaults to ARIACTL_TOKEN")
	rootCmd.PersistentFlags().StringVar(&tenantID, "tenant-id", os.Getenv("ARIACTL_TENANT_ID"), "Tenant ID for tenant-scoped APIs; defaults to ARIACTL_TENANT_ID or token tid claim")

	rootCmd.AddCommand(networkCmd)
	networkCmd.AddCommand(networkListCmd)
	networkCmd.AddCommand(networkAddCmd)
	networkCmd.AddCommand(networkRemoveCmd)
}

func main() {
	if err := rootCmd.Execute(); err != nil {
		os.Exit(1)
	}
}

type NodeInfo struct {
	PublicKey        string   `json:"public_key"`
	Hostname         string   `json:"hostname"`
	AssignedIP       string   `json:"assigned_ip"`
	AdvertisedRoutes []string `json:"advertised_routes,omitempty"`
}

func runNetworkList(cmd *cobra.Command, args []string) error {
	listURL, err := tenantScopedURL("/nodes")
	if err != nil {
		return err
	}

	resp, err := getJSON(listURL)
	if err != nil {
		return fmt.Errorf("failed to connect to controller: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("controller returned error: %s", string(body))
	}

	nodes, err := decodeNodeListResponse(resp.Body)
	if err != nil {
		return fmt.Errorf("failed to decode response: %w", err)
	}

	// Filter by hostname if provided
	var hostname string
	if len(args) > 0 {
		hostname = args[0]
	}

	fmt.Println("Advertised Network Routes:")
	fmt.Println("──────────────────────────────────────────────────────────")

	totalRoutes := 0
	for _, node := range nodes {
		if hostname != "" && node.Hostname != hostname {
			continue
		}

		if len(node.AdvertisedRoutes) == 0 {
			continue
		}

		fmt.Printf("\n%s (%s):\n", node.Hostname, node.AssignedIP)
		for i, route := range node.AdvertisedRoutes {
			fmt.Printf("  %d. %s\n", i+1, route)
			totalRoutes++
		}
	}

	if totalRoutes == 0 {
		fmt.Println("(no routes configured)")
	}

	fmt.Println()
	fmt.Printf("Total: %d routes across %d nodes\n", totalRoutes, len(nodes))

	return nil
}

func runNetworkAdd(cmd *cobra.Command, args []string) error {
	hostname := args[0]
	cidr := args[1]

	// Prepare request
	reqBody := map[string]interface{}{
		"hostname": hostname,
		"cidr":     cidr,
		"action":   "add",
	}
	if resolvedTenantID := resolveTenantID(); resolvedTenantID != "" {
		reqBody["tenant_id"] = resolvedTenantID
	}

	body, _ := json.Marshal(reqBody)
	resp, err := postJSON(controllerURL+"/api/v2/agents/network", body)
	if err != nil {
		return fmt.Errorf("failed to connect to controller: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		respBody, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("controller returned error: %s", string(respBody))
	}

	fmt.Printf("✓ Added route %s for node %s\n", cidr, hostname)
	fmt.Println()
	fmt.Println("Route will be synchronized to all peers automatically")

	return nil
}

func runNetworkRemove(cmd *cobra.Command, args []string) error {
	hostname := args[0]
	cidr := args[1]

	// Prepare request
	reqBody := map[string]interface{}{
		"hostname": hostname,
		"cidr":     cidr,
		"action":   "remove",
	}
	if resolvedTenantID := resolveTenantID(); resolvedTenantID != "" {
		reqBody["tenant_id"] = resolvedTenantID
	}

	body, _ := json.Marshal(reqBody)
	resp, err := postJSON(controllerURL+"/api/v2/agents/network", body)
	if err != nil {
		return fmt.Errorf("failed to connect to controller: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		respBody, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("controller returned error: %s", string(respBody))
	}

	fmt.Printf("✓ Removed route %s from node %s\n", cidr, hostname)
	fmt.Println()
	fmt.Println("Route will be removed from all peers automatically")

	return nil
}

func postJSON(url string, body []byte) (*http.Response, error) {
	req, err := newAuthenticatedRequest(http.MethodPost, url, bytes.NewReader(body))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", "application/json")
	return http.DefaultClient.Do(req)
}

func getJSON(url string) (*http.Response, error) {
	req, err := newAuthenticatedRequest(http.MethodGet, url, nil)
	if err != nil {
		return nil, err
	}
	return http.DefaultClient.Do(req)
}

func newAuthenticatedRequest(method, url string, body io.Reader) (*http.Request, error) {
	req, err := http.NewRequest(method, url, body)
	if err != nil {
		return nil, err
	}
	if strings.TrimSpace(authToken) != "" {
		req.Header.Set("Authorization", "Bearer "+strings.TrimSpace(authToken))
	}
	return req, nil
}

func tenantScopedURL(path string) (string, error) {
	resolvedTenantID := resolveTenantID()
	if resolvedTenantID == "" {
		return "", fmt.Errorf("tenant ID is required for network list; set --tenant-id, ARIACTL_TENANT_ID, or use a tenant-scoped JWT")
	}
	return strings.TrimRight(controllerURL, "/") + "/api/v2/tenants/" + url.PathEscape(resolvedTenantID) + path, nil
}

func resolveTenantID() string {
	if strings.TrimSpace(tenantID) != "" {
		return strings.TrimSpace(tenantID)
	}
	if envTenantID := strings.TrimSpace(os.Getenv("ARIACTL_TENANT_ID")); envTenantID != "" {
		return envTenantID
	}
	token := strings.TrimSpace(authToken)
	if token == "" {
		return ""
	}
	claims, err := auth.ExtractUserInfo(token)
	if err != nil || claims == nil {
		return ""
	}
	return strings.TrimSpace(claims.TenantID)
}

func decodeNodeListResponse(body io.Reader) ([]NodeInfo, error) {
	var raw json.RawMessage
	if err := json.NewDecoder(body).Decode(&raw); err != nil {
		return nil, err
	}

	var nodes []NodeInfo
	if err := json.Unmarshal(raw, &nodes); err == nil {
		return nodes, nil
	}

	var envelope struct {
		Data json.RawMessage `json:"data"`
	}
	if err := json.Unmarshal(raw, &envelope); err != nil {
		return nil, err
	}
	if len(envelope.Data) == 0 {
		return nil, fmt.Errorf("response data is missing")
	}
	if err := json.Unmarshal(envelope.Data, &nodes); err != nil {
		return nil, err
	}
	return nodes, nil
}
