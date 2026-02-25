package cli

import (
	"fmt"
	"strings"

	"github.com/spf13/cobra"

	"aria/internal/token"
)

var tokenRevokeCmd = &cobra.Command{
	Use:   "revoke <token>",
	Short: "Revoke an enrollment token",
	Long: `Revoke an enrollment token to prevent further use.

Once revoked, the token cannot be used for new agent registrations.
Existing agents that have already registered are not affected.

You can revoke by token string or by ID:
  aria token revoke tk_xxxxxxxx
  aria token revoke --id=<uuid>

Examples:
  aria token revoke tk_a8x9k2m7p3q5n1w4
  aria token revoke --id=12345678`,
	RunE: runTokenRevoke,
}

var tokenRevokeID string

func init() {
	tokenCmd.AddCommand(tokenRevokeCmd)
	tokenRevokeCmd.Flags().StringVar(&tokenRevokeID, "id", "", "Revoke by token ID instead of token string")
}

func runTokenRevoke(cmd *cobra.Command, args []string) error {
	// Validate input
	if tokenRevokeID == "" && len(args) == 0 {
		return fmt.Errorf("token string or --id is required")
	}

	// Connect to database
	db, err := connectToDatabase()
	if err != nil {
		return fmt.Errorf("failed to connect to database: %w", err)
	}
	defer db.Close()

	// Create token store
	store := token.NewStore(db)

	var tokenStr string
	if tokenRevokeID != "" {
		// Revoke by ID
		if err := store.RevokeByID(tokenRevokeID); err != nil {
			return fmt.Errorf("failed to revoke token: %w", err)
		}
		fmt.Printf("\nToken with ID %s has been revoked.\n\n", tokenRevokeID)
	} else {
		// Revoke by token string
		tokenStr = args[0]
		if !strings.HasPrefix(tokenStr, "tk_") {
			return fmt.Errorf("invalid token format: must start with 'tk_'")
		}
		if err := store.Revoke(tokenStr); err != nil {
			return fmt.Errorf("failed to revoke token: %w", err)
		}
		fmt.Printf("\nToken %s has been revoked.\n\n", tokenStr)
	}

	return nil
}
