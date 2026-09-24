package submissions

import (
	"context"
	"encoding/csv"
	"encoding/json"
	"errors"
	"io"
	"strconv"
	"strings"
	"time"

	"github.com/google/uuid"

	"github.com/openforms/openforms/internal/definitions"
)

// ExportCSV writes every submission of the form, oldest first. Columns come from
// the form's current version: id, createdAt, state, formVersion, each form field
// key, then "fields.<key>" for each workflow field. Nothing is written if the
// form lookup fails.
func (s *Service) ExportCSV(ctx context.Context, orgID uuid.UUID, formSlug string, w io.Writer) error {
	form, err := s.defs.GetForm(ctx, orgID, formSlug)
	if errors.Is(err, definitions.ErrNotFound) {
		return ErrNotFound
	}
	if err != nil {
		return err
	}
	dataKeys := make([]string, 0, len(form.Definition.Fields))
	for _, f := range form.Definition.Fields {
		dataKeys = append(dataKeys, f.Key)
	}
	var fieldKeys []string
	if form.WorkflowVersionID != nil {
		wf, err := s.defs.GetWorkflowVersion(ctx, *form.WorkflowVersionID)
		if err != nil {
			return err
		}
		for _, f := range wf.Definition.Fields {
			fieldKeys = append(fieldKeys, f.Key)
		}
	}

	rows, err := s.pool.Query(ctx, `SELECT s.id, s.created_at, s.state, fv.version, s.data, s.fields
		FROM submissions s JOIN form_versions fv ON fv.id = s.form_version_id
		WHERE s.org_id = $1 AND s.form_id = $2
		ORDER BY s.created_at, s.id`, orgID, form.ID)
	if err != nil {
		return err
	}
	defer rows.Close()

	cw := csv.NewWriter(w)
	header := append([]string{"id", "createdAt", "state", "formVersion"}, dataKeys...)
	for _, k := range fieldKeys {
		header = append(header, "fields."+k)
	}
	if err := cw.Write(header); err != nil {
		return err
	}
	for rows.Next() {
		var (
			id                 uuid.UUID
			createdAt          time.Time
			state              string
			version            int
			rawData, rawFields []byte
			data, fields       map[string]any
		)
		if err := rows.Scan(&id, &createdAt, &state, &version, &rawData, &rawFields); err != nil {
			return err
		}
		if err := json.Unmarshal(rawData, &data); err != nil {
			return err
		}
		if err := json.Unmarshal(rawFields, &fields); err != nil {
			return err
		}
		record := []string{id.String(), createdAt.UTC().Format(time.RFC3339), state, strconv.Itoa(version)}
		for _, k := range dataKeys {
			record = append(record, csvSafe(csvValue(data[k])))
		}
		for _, k := range fieldKeys {
			record = append(record, csvSafe(csvValue(fields[k])))
		}
		if err := cw.Write(record); err != nil {
			return err
		}
	}
	if err := rows.Err(); err != nil {
		return err
	}
	cw.Flush()
	return cw.Error()
}

func csvValue(v any) string {
	switch x := v.(type) {
	case nil:
		return ""
	case string:
		return x
	case bool:
		return strconv.FormatBool(x)
	case float64:
		return strconv.FormatFloat(x, 'f', -1, 64)
	case []any:
		parts := make([]string, 0, len(x))
		for _, p := range x {
			parts = append(parts, csvValue(p))
		}
		return strings.Join(parts, ";")
	default:
		b, _ := json.Marshal(x)
		return string(b)
	}
}

// csvSafe neutralises spreadsheet formula injection (OWASP CSV injection).
func csvSafe(s string) string {
	if s != "" && strings.ContainsRune("=+-@\t\r", rune(s[0])) {
		return "'" + s
	}
	return s
}
