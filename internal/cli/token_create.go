package cli

import (
	"database/sql"
	"fmt"
	"os"
	"strings"
	"time"

	_ "github.com/lib/pq"
	"github.com/spf13/cobra"

	"aria/internal/token"
)

var tokenCreateCmd = &cobra.Command{
	Use:   "create",
	Short: "Create a new enrollment token",
	Long: `Create a new enrollment token for agent registration.

The token can be used by agents to join the Aria network. You can control:
  - How many times the token can be used (--uses)
  - How long the token remains valid (--ttl)
  - A descriptive tag for easy identification (--tag)

Examples:
  # Create a one-time token valid for 24 hours
  aria token create --tag="shanghai-office"

  # Create a token that can be used 10 times, valid for 7 days
  aria token create --uses=10 --ttl=168h --tag="batch-deployment"

  # Create a token valid for only 1 hour
  aria token create --uses=1 --ttl=1h --tag="urgent-setup"`,
	RunE: runTokenCreate,
}

var (
	tokenUses     int
	tokenTTL      string
	tokenTag      string
	tokenTenantID string
)

func init() {
	tokenCmd.AddCommand(tokenCreateCmd)
	tokenCreateCmd.Flags().IntVar(&tokenUses, "uses", 1, "Maximum number of times this token can be used")
	tokenCreateCmd.Flags().StringVar(&tokenTTL, "ttl", "24h", "Time to live (e.g., 1h, 24h, 7d, 168h)")
	tokenCreateCmd.Flags().StringVar(&tokenTag, "tag", "", "Description tag for the token")
	tokenCreateCmd.Flags().StringVar(&tokenTenantID, "tenant-id", "", "Tenant UUID to bind this enrollment token to")
}

func runTokenCreate(cmd *cobra.Command, args []string) error {
	// Parse TTL
	ttl, err := time.ParseDuration(tokenTTL)
	if err != nil {
		return fmt.Errorf("invalid TTL format: %w (use formats like 1h, 24h, 168h)", err)
	}

	// Connect to database
	db, err := connectToDatabase()
	if err != nil {
		return fmt.Errorf("failed to connect to database: %w", err)
	}
	defer db.Close()

	// Create token store and ensure migration
	store := token.NewStore(db)
	if err := store.Migrate(); err != nil {
		return fmt.Errorf("failed to migrate database: %w", err)
	}

	tenantID, err := resolveTokenTenantID(db, tokenTenantID)
	if err != nil {
		return err
	}

	// Generate token
	t, err := token.GenerateSimple(tokenTag, tokenUses, ttl)
	if err != nil {
		return fmt.Errorf("failed to generate token: %w", err)
	}
	t.TenantID = tenantID

	// Save to database
	if err := store.Create(t); err != nil {
		return fmt.Errorf("failed to save token: %w", err)
	}

	// Output
	fmt.Printf("\nToken:      %s\n", t.Token)
	if t.Tag != "" {
		fmt.Printf("Tag:        %s\n", t.Tag)
	}
	fmt.Printf("Tenant ID:  %s\n", t.TenantID)
	fmt.Printf("Max Uses:   %d\n", t.MaxUses)
	fmt.Printf("Expires:    %s (%s from now)\n", t.ExpiresAt.Format("2006-01-02 15:04:05"), tokenTTL)
	fmt.Printf("Status:     %s\n", t.Status)
	fmt.Println()

	return nil
}

func resolveTokenTenantID(db *sql.DB, requestedTenantID string) (string, error) {
	requestedTenantID = strings.TrimSpace(requestedTenantID)
	if requestedTenantID != "" {
		var exists bool
		if err := db.QueryRow(`SELECT EXISTS (SELECT 1 FROM tenants WHERE id = $1 AND COALESCE(status, 'active') != 'deleted')`, requestedTenantID).Scan(&exists); err != nil {
			return "", fmt.Errorf("failed to validate tenant: %w", err)
		}
		if !exists {
			return "", fmt.Errorf("tenant %s not found or deleted", requestedTenantID)
		}
		return requestedTenantID, nil
	}

	rows, err := db.Query(`SELECT id::text FROM tenants WHERE COALESCE(status, 'active') = 'active' ORDER BY created_at ASC LIMIT 2`)
	if err != nil {
		return "", fmt.Errorf("failed to list active tenants: %w", err)
	}
	defer rows.Close()

	var tenants []string
	for rows.Next() {
		var id string
		if err := rows.Scan(&id); err != nil {
			return "", fmt.Errorf("failed to read active tenant: %w", err)
		}
		tenants = append(tenants, id)
	}
	if err := rows.Err(); err != nil {
		return "", fmt.Errorf("failed to list active tenants: %w", err)
	}
	if len(tenants) == 1 {
		return tenants[0], nil
	}
	if len(tenants) > 1 {
		return "", fmt.Errorf("multiple active tenants found; pass --tenant-id explicitly")
	}

	var id string
	if err := db.QueryRow(
		`INSERT INTO tenants (code, name) VALUES ($1, $2) ON CONFLICT (code) DO UPDATE SET name = $2 RETURNING id::text`,
		"default", "default",
	).Scan(&id); err != nil {
		return "", fmt.Errorf("failed to create default tenant: %w", err)
	}
	return id, nil
}

// connectToDatabase creates a database connection using environment variables or config
func connectToDatabase() (*sql.DB, error) {
	// Try environment variables first
	host := os.Getenv("POSTGRES_HOST")
	if host == "" {
		host = "localhost"
	}
	port := os.Getenv("POSTGRES_PORT")
	if port == "" {
		port = "5432"
	}
	user := os.Getenv("POSTGRES_USER")
	if user == "" {
		user = "postgres"
	}
	password := os.Getenv("POSTGRES_PASSWORD")
	database := os.Getenv("POSTGRES_DATABASE")
	if database == "" {
		database = "aria"
	}
	sslmode := os.Getenv("POSTGRES_SSLMODE")
	if sslmode == "" {
		sslmode = "disable"
	}

	dsn := fmt.Sprintf(
		"host=%s port=%s user=%s password=%s dbname=%s sslmode=%s",
		host, port, user, password, database, sslmode,
	)

	db, err := sql.Open("postgres", dsn)
	if err != nil {
		return nil, err
	}

	if err := db.Ping(); err != nil {
		return nil, fmt.Errorf("cannot connect to PostgreSQL at %s:%s: %w", host, port, err)
	}

	return db, nil
}
