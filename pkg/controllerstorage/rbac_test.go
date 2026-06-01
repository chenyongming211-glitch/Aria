package controllerstorage

import "testing"

func TestNormalizeRoleNameMapsLegacyTenantRoles(t *testing.T) {
	cases := map[string]string{
		"member":      SystemRoleOperator,
		" owner ":     SystemRoleAdmin,
		"ADMIN":       SystemRoleAdmin,
		"SUPER_ADMIN": "super_admin",
		"custom":      "custom",
	}

	for input, want := range cases {
		if got := NormalizeRoleName(input); got != want {
			t.Fatalf("NormalizeRoleName(%q) = %q, want %q", input, got, want)
		}
	}
}
