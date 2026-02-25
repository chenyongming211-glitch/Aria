package cli

import (
	"github.com/spf13/cobra"
)

var tokenCmd = &cobra.Command{
	Use:   "token",
	Short: "Manage enrollment tokens",
	Long: `Manage enrollment tokens for agent registration.

Tokens control who can join the Aria network. Each token has:
  - Maximum uses: How many agents can use this token
  - TTL: How long the token remains valid
  - Tag: A description for easy identification

Examples:
  aria token create --uses=1 --ttl=24h --tag="shanghai-office"
  aria token list
  aria token revoke tk_xxxxxxxx`,
}

func init() {
	rootCmd.AddCommand(tokenCmd)
}
