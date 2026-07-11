package handlers

import (
	"net/url"
	"reflect"
	"strings"
	"testing"
)

func TestParseFinanceTransactionQueryPagination(t *testing.T) {
	tests := []struct {
		name         string
		rawQuery     string
		wantPage     int
		wantPageSize int
	}{
		{name: "defaults", wantPage: 1, wantPageSize: 30},
		{name: "valid values", rawQuery: "page=2&page_size=75", wantPage: 2, wantPageSize: 75},
		{name: "invalid values use defaults", rawQuery: "page=invalid&page_size=invalid", wantPage: 1, wantPageSize: 30},
		{name: "non-positive values use defaults", rawQuery: "page=-3&page_size=0", wantPage: 1, wantPageSize: 30},
		{name: "page size is capped", rawQuery: "page=3&page_size=999", wantPage: 3, wantPageSize: 250},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			params := parseFinanceTransactionQuery(mustParseFinanceQuery(t, tt.rawQuery))
			if params.Page != tt.wantPage {
				t.Fatalf("Page = %d, want %d", params.Page, tt.wantPage)
			}
			if params.PageSize != tt.wantPageSize {
				t.Fatalf("PageSize = %d, want %d", params.PageSize, tt.wantPageSize)
			}
		})
	}
}

func TestBuildFinanceTransactionQuery(t *testing.T) {
	tests := []struct {
		name        string
		rawQuery    string
		wantArgs    []any
		wantSQL     []string
		unwantedSQL []string
	}{
		{
			name:     "defaults",
			wantArgs: []any{"user-1", 30, 0},
			wantSQL: []string{
				"WHERE t.is_transfer = false AND t.user_id = $1 AND COALESCE(a.in_balance, TRUE) = TRUE",
				"ORDER BY t.occurred_at DESC",
				"LIMIT $2 OFFSET $3",
			},
			unwantedSQL: []string{"t.amount > 0", "t.amount < 0", "ILIKE"},
		},
		{
			name:        "income filter",
			rawQuery:    "type=income",
			wantArgs:    []any{"user-1", 30, 0},
			wantSQL:     []string{"t.amount > 0", "LIMIT $2 OFFSET $3"},
			unwantedSQL: []string{"t.amount < 0"},
		},
		{
			name:        "expense filter",
			rawQuery:    "type=expense",
			wantArgs:    []any{"user-1", 30, 0},
			wantSQL:     []string{"t.amount < 0", "LIMIT $2 OFFSET $3"},
			unwantedSQL: []string{"t.amount > 0"},
		},
		{
			name:     "filters preserve placeholder and argument order",
			rawQuery: "type=expense&category=Dining&payee=Cafe&search=latte&from=2026-01-01&to=2026-01-31&sort=category&order=asc&page=2&page_size=50",
			wantArgs: []any{
				"user-1",
				"Dining",
				"Cafe",
				"%latte%",
				"2026-01-01",
				"2026-01-31",
				50,
				50,
			},
			wantSQL: []string{
				"t.amount < 0",
				"t.category = $2",
				"COALESCE(NULLIF(t.payee,''), NULLIF(t.comment,''), 'Без описания') = $3",
				"(COALESCE(t.comment,'') ILIKE $4 OR COALESCE(t.payee,'') ILIKE $4 OR COALESCE(t.category,'') ILIKE $4 OR COALESCE(a.title,'') ILIKE $4)",
				"t.occurred_at >= $5",
				"t.occurred_at < ($6::date + INTERVAL '1 day')",
				"ORDER BY t.category ASC, t.occurred_at DESC",
				"LIMIT $7 OFFSET $8",
			},
		},
		{
			name:     "uncategorized sentinel does not add argument",
			rawQuery: "category=__uncategorized__&search=tea",
			wantArgs: []any{"user-1", "%tea%", 30, 0},
			wantSQL: []string{
				"NULLIF(TRIM(t.category), '') IS NULL",
				"ILIKE $2",
				"LIMIT $3 OFFSET $4",
			},
			unwantedSQL: []string{"t.category = $2"},
		},
		{
			name:        "localized uncategorized value does not add argument",
			rawQuery:    "category=%D0%91%D0%B5%D0%B7+%D0%BA%D0%B0%D1%82%D0%B5%D0%B3%D0%BE%D1%80%D0%B8%D0%B8",
			wantArgs:    []any{"user-1", 30, 0},
			wantSQL:     []string{"NULLIF(TRIM(t.category), '') IS NULL", "LIMIT $2 OFFSET $3"},
			unwantedSQL: []string{"t.category = $2"},
		},
		{
			name:     "page size cap affects limit and offset arguments",
			rawQuery: "page=3&page_size=999",
			wantArgs: []any{"user-1", 250, 500},
			wantSQL:  []string{"LIMIT $2 OFFSET $3"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			params := parseFinanceTransactionQuery(mustParseFinanceQuery(t, tt.rawQuery))
			query := buildFinanceTransactionQuery("user-1", params)

			if !reflect.DeepEqual(query.Args, tt.wantArgs) {
				t.Fatalf("Args = %#v, want %#v", query.Args, tt.wantArgs)
			}
			for _, fragment := range tt.wantSQL {
				if !strings.Contains(query.SQL, fragment) {
					t.Errorf("SQL does not contain %q:\n%s", fragment, query.SQL)
				}
			}
			for _, fragment := range tt.unwantedSQL {
				if strings.Contains(query.SQL, fragment) {
					t.Errorf("SQL unexpectedly contains %q:\n%s", fragment, query.SQL)
				}
			}
		})
	}
}

func TestFinanceTransactionOrderBy(t *testing.T) {
	tests := []struct {
		name     string
		rawQuery string
		want     string
	}{
		{name: "default", want: "t.occurred_at DESC"},
		{name: "default ignores direction", rawQuery: "order=asc", want: "t.occurred_at DESC"},
		{name: "amount descending", rawQuery: "sort=amount", want: "ABS(t.amount) DESC"},
		{name: "amount ascending", rawQuery: "sort=amount&order=asc", want: "ABS(t.amount) ASC"},
		{name: "signed amount ascending", rawQuery: "sort=signed_amount&order=asc", want: "t.amount ASC"},
		{name: "legacy date ascending", rawQuery: "sort=date_asc&order=desc", want: "t.occurred_at ASC"},
		{name: "legacy amount ascending", rawQuery: "sort=amount_asc&order=desc", want: "ABS(t.amount) ASC"},
		{name: "date with direction", rawQuery: "sort=date&order=asc", want: "t.occurred_at ASC"},
		{name: "occurred at with direction", rawQuery: "sort=occurred_at&order=asc", want: "t.occurred_at ASC"},
		{name: "category with tie breaker", rawQuery: "sort=category&order=asc", want: "t.category ASC, t.occurred_at DESC"},
		{name: "payee descending", rawQuery: "sort=payee", want: "t.payee DESC, t.occurred_at DESC"},
		{name: "unknown sort uses default", rawQuery: "sort=unknown&order=asc", want: "t.occurred_at DESC"},
		{name: "direction is case sensitive", rawQuery: "sort=amount&order=ASC", want: "ABS(t.amount) DESC"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			params := parseFinanceTransactionQuery(mustParseFinanceQuery(t, tt.rawQuery))
			if got := financeTransactionOrderBy(params); got != tt.want {
				t.Fatalf("financeTransactionOrderBy() = %q, want %q", got, tt.want)
			}
		})
	}
}

func mustParseFinanceQuery(t *testing.T, rawQuery string) url.Values {
	t.Helper()
	values, err := url.ParseQuery(rawQuery)
	if err != nil {
		t.Fatalf("parse query %q: %v", rawQuery, err)
	}
	return values
}
