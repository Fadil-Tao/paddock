package repository

import (
	"fmt"
	"reflect"
	"strings"
	"time"

	"github.com/Fadil-Tao/paddock/internal/model"
)

func parseSQLiteTime(value string) (time.Time, error) {
	value = strings.TrimSpace(value)
	if value == "" {
		return time.Time{}, nil
	}

	layouts := []string{
		time.RFC3339Nano,
		time.RFC3339,
		"2006-01-02 15:04:05.999999999 -0700 MST",
		"2006-01-02 15:04:05 -0700 MST",
		"2006-01-02 15:04:05.999999999-07:00",
		"2006-01-02 15:04:05.999999999Z07:00",
		"2006-01-02 15:04:05.999999999",
		"2006-01-02 15:04:05",
	}

	var lastErr error
	for _, layout := range layouts {
		parsed, err := time.Parse(layout, value)
		if err == nil {
			return parsed, nil
		}
		lastErr = err
	}

	return time.Time{}, lastErr
}

func searchParams(query string, params *model.SearchParams) (string, string, int, int) {
	keyword := ""
	if params != nil && strings.TrimSpace(params.Keyword) != "" {
		query += ` WHERE id LIKE ? OR name LIKE ? OR image LIKE ?`
		keyword = "%" + strings.TrimSpace(params.Keyword) + "%"
	}

	query += ` ORDER BY created_at DESC`

	limit := 0
	offset := 0
	if params != nil && params.Limit > 0 {
		page := params.Page
		if page < 1 {
			page = 1
		}

		limit = params.Limit
		offset = (page - 1) * params.Limit
		query += ` LIMIT ? OFFSET ?`
	}

	return query, keyword, limit, offset
}

func buildPatchQuery(table string, idColumn string, id string, patch any) (string, []any, error) {
	value := reflect.ValueOf(patch)
	if value.Kind() != reflect.Ptr || value.IsNil() {
		return "", nil, nil
	}

	value = value.Elem()
	if value.Kind() != reflect.Struct {
		return "", nil, fmt.Errorf("patch must be a struct pointer")
	}

	typ := value.Type()
	setParts := make([]string, 0, value.NumField())
	args := make([]any, 0, value.NumField()+1)

	for i := 0; i < value.NumField(); i++ {
		field := value.Field(i)
		if field.Kind() != reflect.Ptr || field.IsNil() {
			continue
		}

		column := typ.Field(i).Tag.Get("db")
		if column == "" || column == "-" {
			continue
		}

		setParts = append(setParts, column+" = ?")
		args = append(args, field.Elem().Interface())
	}

	if len(setParts) == 0 {
		return "", nil, nil
	}

	args = append(args, id)
	query := fmt.Sprintf("UPDATE %s SET %s WHERE %s = ?", table, strings.Join(setParts, ", "), idColumn)

	return query, args, nil
}
