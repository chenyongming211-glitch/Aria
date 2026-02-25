package cli

import (
	"fmt"
	"strings"
	"time"

	"github.com/spf13/cobra"

	"aria/internal/token"
)

var tokenListCmd = &cobra.Command{
	Use:   "list",
	Short: "List all enrollment tokens",
	Long: `List all enrollment tokens with their status and usage information.

By default, only active tokens are shown. Use --status to filter:
  --status=active    Show only active tokens (default)
  --status=all       Show all tokens
  --status=expired   Show only expired tokens
  --status=exhausted Show only exhausted tokens
  --status=revoked   Show only revoked tokens

Examples:
  aria token list
  aria token list --status=all`,
	RunE: runTokenList,
}

var tokenListStatus string

func init() {
	tokenCmd.AddCommand(tokenListCmd)
	tokenListCmd.Flags().StringVar(&tokenListStatus, "status", "active", "Filter by status (active, all, expired, exhausted, revoked)")
}

func runTokenList(cmd *cobra.Command, args []string) error {
	// Connect to database
	db, err := connectToDatabase()
	if err != nil {
		return fmt.Errorf("failed to connect to database: %w", err)
	}
	defer db.Close()

	// Create token store
	store := token.NewStore(db)

	// List tokens
	var status token.Status
	if tokenListStatus != "all" {
		status = token.Status(tokenListStatus)
	}

	tokens, err := store.List(status)
	if err != nil {
		return fmt.Errorf("failed to list tokens: %w", err)
	}

	if len(tokens) == 0 {
		fmt.Println("No tokens found.")
		return nil
	}

	// Print header
	fmt.Println()
	fmt.Printf("%-8s  %-20s  %-16s  %-8s  %-20s  %-10s\n",
		"ID", "TOKEN", "TAG", "USES", "EXPIRES", "STATUS")
	fmt.Println(strings.Repeat("-", 90))

	// Print tokens
	for _, t := range tokens {
		// Truncate ID and token for display
		idShort := t.ID.String()
		if len(idShort) > 8 {
			idShort = idShort[:8]
		}
		tokenShort := t.Token
		if len(tokenShort) > 16 {
			tokenShort = tokenShort[:12] + "..." + tokenShort[len(tokenShort)-4:]
		}

		tag := t.Tag
		if len(tag) > 16 {
			tag = tag[:13] + "..."
		}
		if tag == "" {
			tag = "-"
		}

		uses := fmt.Sprintf("%d/%d", t.UsedCount, t.MaxUses)

		expires := t.ExpiresAt.Format("2006-01-02 15:04")
		if t.ExpiresAt.Before(time.Now()) {
			expires = expires + " (expired)"
		}

		statusStr := string(t.Status)

		fmt.Printf("%-8s  %-20s  %-16s  %-8s  %-20s  %-10s\n",
			idShort, tokenShort, tag, uses, expires, statusStr)
	}

	fmt.Println()
	fmt.Printf("Total: %d token(s)\n", len(tokens))
	fmt.Println()

	return nil
}
