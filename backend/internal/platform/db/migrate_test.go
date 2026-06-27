package db

import "testing"

func TestToMigrateDSN(t *testing.T) {
	cases := []struct {
		name string
		in   string
		want string
	}{
		{"postgres scheme", "postgres://u:p@h:5432/c5?sslmode=disable", "pgx5://u:p@h:5432/c5?sslmode=disable"},
		{"postgresql scheme", "postgresql://u:p@h:5432/c5", "pgx5://u:p@h:5432/c5"},
		{"already pgx5 passthrough", "pgx5://u:p@h/c5", "pgx5://u:p@h/c5"},
		{"keyword=value passthrough", "host=h user=u dbname=c5", "host=h user=u dbname=c5"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := toMigrateDSN(tc.in); got != tc.want {
				t.Fatalf("toMigrateDSN(%q) = %q, want %q", tc.in, got, tc.want)
			}
		})
	}
}
