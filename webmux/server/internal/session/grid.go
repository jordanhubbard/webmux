package session

import (
	"fmt"
	"math"
	"os"
	"slices"
	"strings"

	"github.com/jordanhubbard/webmux/server/internal/config"
	"github.com/jordanhubbard/webmux/server/internal/storage"
)

type limits struct{ cols, rows *float64 }

func invalid(message string) error { return &config.ValidationError{Message: message} }

func loadLimits(store *storage.Store) (limits, error) {
	var document config.Document
	// Legacy installations without app.yaml have an unlimited grid. Invalid
	// configured limits, however, must not silently become unlimited.
	_ = store.ReadConfig("app.yaml", &document)
	raw := config.AsObject(document.App["terminal_grid"])
	var result limits
	for _, field := range []string{"cols", "rows"} {
		name := "max_" + field
		value, err := config.GridLimit(raw[name], "app.terminal_grid."+name)
		if err != nil {
			return result, err
		}
		env := "WEBMUX_TERMINAL_GRID_" + strings.ToUpper(name)
		if override, ok := os.LookupEnv(env); ok {
			value, err = config.GridLimit(override, env)
			if err != nil {
				return result, err
			}
		}
		if field == "cols" {
			result.cols = value
		} else {
			result.rows = value
		}
	}
	return result, nil
}
func (l limits) validate(row, col int) error {
	if row < 0 || col < 0 {
		return invalid("Terminal grid row and col must be non-negative integers")
	}
	if l.rows != nil && float64(row) >= *l.rows {
		return invalid(fmt.Sprintf("Terminal grid row %d exceeds max_rows %g", row, *l.rows))
	}
	if l.cols != nil && float64(col) >= *l.cols {
		return invalid(fmt.Sprintf("Terminal grid column %d exceeds max_cols %g", col, *l.cols))
	}
	return nil
}
func (l limits) next(items []*entry, row, col *int) (int, int, error) {
	if row != nil && col != nil {
		return *row, *col, l.validate(*row, *col)
	}
	if len(items) == 0 {
		return 0, 0, l.validate(0, 0)
	}
	if l.cols != nil {
		// Only occupied positions can postpone the first free cell. Sorting those
		// positions avoids a scan proportional to a user-configured grid limit.
		positions := make([][2]int, 0, len(items))
		for _, item := range items {
			if l.validate(item.value.Row, item.value.Col) == nil {
				positions = append(positions, [2]int{item.value.Row, item.value.Col})
			}
		}
		slices.SortFunc(positions, func(a, b [2]int) int {
			if a[0] < b[0] || a[0] == b[0] && a[1] < b[1] {
				return -1
			}
			if a == b {
				return 0
			}
			return 1
		})
		r, c := 0, 0
		for _, position := range positions {
			if position[0] == r && position[1] == c {
				if c == math.MaxInt {
					return 0, 0, invalid("Terminal grid is full")
				}
				c++
				if float64(c) >= *l.cols {
					c = 0
					r++
				}
			} else if position[0] > r || position[0] == r && position[1] > c {
				break
			}
		}
		if err := l.validate(r, c); err != nil {
			return 0, 0, invalid("Terminal grid is full")
		}
		return r, c, nil
	}
	r, c := -1, -1
	for _, item := range items {
		s := item.value
		if l.rows != nil && float64(s.Row) >= *l.rows {
			continue
		}
		if s.Row > r {
			r, c = s.Row, s.Col
		} else if s.Row == r && s.Col > c {
			c = s.Col
		}
	}
	if r < 0 {
		return 0, 0, l.validate(0, 0)
	}
	if c == math.MaxInt {
		return 0, 0, invalid("Terminal grid is full")
	}
	return r, c + 1, l.validate(r, c+1)
}
func compact(items []*entry) {
	slices.SortStableFunc(items, func(a, b *entry) int {
		if a.value.Row < b.value.Row || a.value.Row == b.value.Row && a.value.Col < b.value.Col {
			return -1
		}
		if a.value.Row == b.value.Row && a.value.Col == b.value.Col {
			return 0
		}
		return 1
	})
	row, col, previous := -1, 0, -1
	for _, item := range items {
		old := item.value.Row
		if row < 0 || old != previous {
			row++
			col = 0
		}
		item.value.Row, item.value.Col = row, col
		previous = old
		col++
	}
}
