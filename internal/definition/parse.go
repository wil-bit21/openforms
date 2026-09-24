package definition

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"sort"
	"strconv"
	"strings"
	"sync"

	"github.com/santhosh-tekuri/jsonschema/v6"
	"golang.org/x/text/language"
	"golang.org/x/text/message"
	"sigs.k8s.io/yaml"

	"github.com/openforms/openforms/schemas"
)

const (
	formSchemaID     = "https://openforms.dev/schemas/form.schema.json"
	workflowSchemaID = "https://openforms.dev/schemas/workflow.schema.json"
)

var (
	schemaOnce     sync.Once
	schemaErr      error
	formSchema     *jsonschema.Schema
	workflowSchema *jsonschema.Schema
	printer        = message.NewPrinter(language.English)
)

func loadSchemas() error {
	schemaOnce.Do(func() {
		c := jsonschema.NewCompiler()
		for id, file := range map[string]string{formSchemaID: "form.schema.json", workflowSchemaID: "workflow.schema.json"} {
			b, err := schemas.FS.ReadFile(file)
			if err != nil {
				schemaErr = err
				return
			}
			doc, err := jsonschema.UnmarshalJSON(bytes.NewReader(b))
			if err != nil {
				schemaErr = fmt.Errorf("%s: %w", file, err)
				return
			}
			if err := c.AddResource(id, doc); err != nil {
				schemaErr = err
				return
			}
		}
		if formSchema, schemaErr = c.Compile(formSchemaID); schemaErr != nil {
			return
		}
		workflowSchema, schemaErr = c.Compile(workflowSchemaID)
	})
	return schemaErr
}

// ParseForm parses a YAML or JSON form definition, validates it against
// form.schema.json and then against the semantic rules. Invalid input yields
// *ValidationError.
func ParseForm(raw []byte) (Form, error) {
	var f Form
	if err := decodeDocument(raw, formSchemaID, &f); err != nil {
		return Form{}, err
	}
	if err := ValidateForm(f); err != nil {
		return Form{}, err
	}
	return f, nil
}

// ParseWorkflow is ParseForm for workflow definitions.
func ParseWorkflow(raw []byte) (Workflow, error) {
	var w Workflow
	if err := decodeDocument(raw, workflowSchemaID, &w); err != nil {
		return Workflow{}, err
	}
	if err := ValidateWorkflow(w); err != nil {
		return Workflow{}, err
	}
	return w, nil
}

func rootProblem(msg string) error {
	return &ValidationError{Problems: []Problem{{Path: "", Message: msg}}}
}

func decodeDocument(raw []byte, schemaID string, out any) error {
	if err := loadSchemas(); err != nil {
		return fmt.Errorf("load schemas: %w", err)
	}
	if len(bytes.TrimSpace(raw)) == 0 {
		return rootProblem("document is empty")
	}
	// Strict mode rejects duplicate keys instead of letting the last one win.
	js, err := yaml.YAMLToJSONStrict(raw)
	if err != nil {
		return rootProblem("invalid YAML/JSON: " + err.Error())
	}
	inst, err := jsonschema.UnmarshalJSON(bytes.NewReader(js))
	if err != nil {
		return rootProblem("invalid JSON: " + err.Error())
	}
	sch := formSchema
	if schemaID == workflowSchemaID {
		sch = workflowSchema
	}
	if err := sch.Validate(inst); err != nil {
		var ve *jsonschema.ValidationError
		if errors.As(err, &ve) {
			return &ValidationError{Problems: schemaProblems(ve)}
		}
		return err
	}
	if err := json.Unmarshal(js, out); err != nil {
		return rootProblem("invalid document: " + err.Error())
	}
	return nil
}

// schemaProblems flattens a jsonschema error tree into one Problem per leaf.
func schemaProblems(ve *jsonschema.ValidationError) []Problem {
	var out []Problem
	seen := map[Problem]bool{}
	var walk func(e *jsonschema.ValidationError)
	walk = func(e *jsonschema.ValidationError) {
		if len(e.Causes) == 0 {
			p := Problem{Path: instancePath(e.InstanceLocation), Message: e.ErrorKind.LocalizedString(printer)}
			if !seen[p] {
				seen[p] = true
				out = append(out, p)
			}
			return
		}
		for _, c := range e.Causes {
			walk(c)
		}
	}
	walk(ve)
	sort.SliceStable(out, func(i, j int) bool { return out[i].Path < out[j].Path })
	return out
}

// instancePath turns ["fields","0","showIf"] into "fields[0].showIf".
// Numeric tokens are array indices: object keys can never be all-digit
// because every key in the schemas matches a letter-first pattern.
func instancePath(tokens []string) string {
	var b strings.Builder
	for _, t := range tokens {
		if _, err := strconv.Atoi(t); err == nil {
			b.WriteString("[" + t + "]")
			continue
		}
		if b.Len() > 0 {
			b.WriteByte('.')
		}
		b.WriteString(t)
	}
	return b.String()
}
