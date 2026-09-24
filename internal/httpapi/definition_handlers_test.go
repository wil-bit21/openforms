package httpapi_test

import (
	"context"
	"net/http"
	"strings"
	"testing"

	"github.com/openforms/openforms/internal/testutil"
)

type itemResp struct {
	Item struct {
		Kind    string `json:"kind"`
		Slug    string `json:"slug"`
		Version int    `json:"version"`
		Changed bool   `json:"changed"`
		Created bool   `json:"created"`
	} `json:"item"`
}

type errResp struct {
	Error struct {
		Code    string `json:"code"`
		Details []struct {
			Path    string `json:"path"`
			Message string `json:"message"`
		} `json:"details"`
	} `json:"error"`
}

func TestPutDefinitionsRequireAdmin(t *testing.T) {
	a := newDefsSubsAPI(t)
	reviewer := a.env.APIKey(t, "reviewer")
	rec := testutil.Do(t, a.h, "PUT", "/api/v1/workflows/review", reviewer, testutil.SampleWorkflow("review"))
	if rec.Code != http.StatusForbidden || testutil.ErrorCode(t, rec) != "forbidden" {
		t.Fatalf("reviewer PUT: %d %s", rec.Code, rec.Body)
	}
	rec = testutil.Do(t, a.h, "PUT", "/api/v1/workflows/review", "", testutil.SampleWorkflow("review"))
	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("anonymous PUT: %d", rec.Code)
	}
}

func TestPutWorkflowThenForm(t *testing.T) {
	a := newDefsSubsAPI(t)
	rec := testutil.Do(t, a.h, "PUT", "/api/v1/workflows/review", a.admin, testutil.SampleWorkflow("review"))
	if rec.Code != http.StatusOK {
		t.Fatalf("PUT workflow: %d %s", rec.Code, rec.Body)
	}
	got := testutil.Decode[itemResp](t, rec)
	if got.Item.Kind != "workflow" || got.Item.Version != 1 || !got.Item.Created || !got.Item.Changed {
		t.Fatalf("item = %+v", got.Item)
	}
	rec = testutil.Do(t, a.h, "PUT", "/api/v1/forms/contact?source=ui", a.admin, testutil.SampleForm("contact", "review"))
	if rec.Code != http.StatusOK {
		t.Fatalf("PUT form: %d %s", rec.Code, rec.Body)
	}
	rec = testutil.Do(t, a.h, "PUT", "/api/v1/forms/contact?source=ui", a.admin, testutil.SampleForm("contact", "review"))
	got = testutil.Decode[itemResp](t, rec)
	if got.Item.Changed || got.Item.Version != 1 {
		t.Fatalf("idempotent PUT item = %+v", got.Item)
	}
	f, err := a.defs.GetForm(context.Background(), a.env.OrgID, "contact")
	if err != nil {
		t.Fatal(err)
	}
	if f.Current.Source != "ui" || f.Current.CreatedBy == "" {
		t.Fatalf("version info = %+v", f.Current)
	}
}

func TestPutFormSlugMismatch(t *testing.T) {
	a := newDefsSubsAPI(t)
	rec := testutil.Do(t, a.h, "PUT", "/api/v1/forms/other", a.admin, testutil.SampleForm("plain", ""))
	if rec.Code != http.StatusUnprocessableEntity {
		t.Fatalf("status = %d %s", rec.Code, rec.Body)
	}
	e := testutil.Decode[errResp](t, rec)
	if e.Error.Code != "validation_failed" || len(e.Error.Details) != 1 || e.Error.Details[0].Path != "slug" {
		t.Fatalf("error = %+v", e.Error)
	}
}

func TestPutFormInvalidDefinition(t *testing.T) {
	a := newDefsSubsAPI(t)
	body := map[string]any{
		"slug": "plain", "title": "Bad", "settings": map[string]any{"public": true},
		"fields": []any{map[string]any{"key": "1bad", "type": "nope", "label": "X"}},
	}
	rec := testutil.Do(t, a.h, "PUT", "/api/v1/forms/plain", a.admin, body)
	if rec.Code != http.StatusUnprocessableEntity || testutil.ErrorCode(t, rec) != "validation_failed" {
		t.Fatalf("status = %d %s", rec.Code, rec.Body)
	}
}

func TestPutFormMissingWorkflow(t *testing.T) {
	a := newDefsSubsAPI(t)
	rec := testutil.Do(t, a.h, "PUT", "/api/v1/forms/contact", a.admin, testutil.SampleForm("contact", "nope"))
	if rec.Code != http.StatusUnprocessableEntity {
		t.Fatalf("status = %d %s", rec.Code, rec.Body)
	}
}

func TestPutFormUnknownSource(t *testing.T) {
	a := newDefsSubsAPI(t)
	rec := testutil.Do(t, a.h, "PUT", "/api/v1/forms/plain?source=seed", a.admin, testutil.SampleForm("plain", ""))
	if rec.Code != http.StatusBadRequest || testutil.ErrorCode(t, rec) != "bad_request" {
		t.Fatalf("status = %d %s", rec.Code, rec.Body)
	}
}

func TestGetFormAndList(t *testing.T) {
	a := newDefsSubsAPI(t)
	a.seed(t)
	a.create(t, "contact")
	a.create(t, "contact")
	reviewer := a.env.APIKey(t, "reviewer")

	rec := testutil.Do(t, a.h, "GET", "/api/v1/forms/contact", reviewer, nil)
	if rec.Code != http.StatusOK {
		t.Fatalf("GET form: %d %s", rec.Code, rec.Body)
	}
	form := testutil.Decode[struct {
		Form struct {
			Slug            string `json:"slug"`
			Version         int    `json:"version"`
			Source          string `json:"source"`
			UpdatedAt       string `json:"updatedAt"`
			WorkflowVersion *int   `json:"workflowVersion"`
			Definition      struct {
				Title  string `json:"title"`
				Fields []any  `json:"fields"`
			} `json:"definition"`
		} `json:"form"`
	}](t, rec)
	if form.Form.Slug != "contact" || form.Form.Version != 1 || form.Form.Source != "api" ||
		form.Form.WorkflowVersion == nil || *form.Form.WorkflowVersion != 1 || form.Form.Definition.Title != "Contact" ||
		!strings.HasSuffix(form.Form.UpdatedAt, "Z") {
		t.Fatalf("form = %+v", form.Form)
	}

	rec = testutil.Do(t, a.h, "GET", "/api/v1/forms/missing", reviewer, nil)
	if rec.Code != http.StatusNotFound || testutil.ErrorCode(t, rec) != "not_found" {
		t.Fatalf("missing form: %d", rec.Code)
	}

	rec = testutil.Do(t, a.h, "GET", "/api/v1/forms", reviewer, nil)
	list := testutil.Decode[struct {
		Items []struct {
			Slug            string  `json:"slug"`
			Title           string  `json:"title"`
			Workflow        *string `json:"workflow"`
			Public          bool    `json:"public"`
			Version         int     `json:"version"`
			Source          string  `json:"source"`
			SubmissionCount int     `json:"submissionCount"`
		} `json:"items"`
	}](t, rec)
	if len(list.Items) != 3 || list.Items[0].Slug != "contact" || list.Items[0].SubmissionCount != 2 ||
		list.Items[0].Workflow == nil || *list.Items[0].Workflow != "review" || !list.Items[0].Public {
		t.Fatalf("list = %+v", list.Items)
	}
	if list.Items[1].Slug != "internal" || list.Items[1].Public || list.Items[2].Workflow != nil || list.Items[2].SubmissionCount != 0 {
		t.Fatalf("list = %+v", list.Items)
	}
}

func TestFormVersionEndpoints(t *testing.T) {
	a := newDefsSubsAPI(t)
	testutil.Do(t, a.h, "PUT", "/api/v1/forms/plain", a.admin, testutil.SampleForm("plain", ""))
	f := testutil.SampleForm("plain", "")
	f.Title = "Contact v2"
	testutil.Do(t, a.h, "PUT", "/api/v1/forms/plain", a.admin, f)

	rec := testutil.Do(t, a.h, "GET", "/api/v1/forms/plain/versions", a.admin, nil)
	vs := testutil.Decode[struct {
		Items []struct {
			Version   int    `json:"version"`
			Hash      string `json:"hash"`
			Source    string `json:"source"`
			CreatedBy string `json:"createdBy"`
			CreatedAt string `json:"createdAt"`
		} `json:"items"`
	}](t, rec)
	if len(vs.Items) != 2 || vs.Items[0].Version != 2 || len(vs.Items[0].Hash) != 64 {
		t.Fatalf("versions = %+v", vs.Items)
	}

	rec = testutil.Do(t, a.h, "GET", "/api/v1/forms/plain/versions/1", a.admin, nil)
	v1 := testutil.Decode[struct {
		Form struct {
			Version    int `json:"version"`
			Definition struct {
				Title string `json:"title"`
			} `json:"definition"`
		} `json:"form"`
	}](t, rec)
	if v1.Form.Version != 1 || v1.Form.Definition.Title != "Contact" {
		t.Fatalf("v1 = %+v", v1.Form)
	}
	for _, path := range []string{"/api/v1/forms/plain/versions/9", "/api/v1/forms/plain/versions/abc", "/api/v1/forms/missing/versions"} {
		if rec := testutil.Do(t, a.h, "GET", path, a.admin, nil); rec.Code != http.StatusNotFound {
			t.Fatalf("%s: %d", path, rec.Code)
		}
	}
}

func TestWorkflowEndpoints(t *testing.T) {
	a := newDefsSubsAPI(t)
	a.seed(t)
	rec := testutil.Do(t, a.h, "GET", "/api/v1/workflows", a.admin, nil)
	list := testutil.Decode[struct {
		Items []struct {
			Slug       string `json:"slug"`
			Title      string `json:"title"`
			Version    int    `json:"version"`
			StateCount int    `json:"stateCount"`
		} `json:"items"`
	}](t, rec)
	if len(list.Items) != 1 || list.Items[0].Slug != "review" || list.Items[0].StateCount != 3 || list.Items[0].Title != "Review" {
		t.Fatalf("workflows = %+v", list.Items)
	}
	rec = testutil.Do(t, a.h, "GET", "/api/v1/workflows/review", a.admin, nil)
	wf := testutil.Decode[struct {
		Workflow struct {
			Slug       string `json:"slug"`
			Definition struct {
				Initial string `json:"initial"`
			} `json:"definition"`
		} `json:"workflow"`
	}](t, rec)
	if wf.Workflow.Slug != "review" || wf.Workflow.Definition.Initial != "new" {
		t.Fatalf("workflow = %+v", wf.Workflow)
	}
	if rec := testutil.Do(t, a.h, "GET", "/api/v1/workflows/review/versions", a.admin, nil); rec.Code != http.StatusOK {
		t.Fatalf("versions: %d", rec.Code)
	}
	if rec := testutil.Do(t, a.h, "GET", "/api/v1/workflows/review/versions/1", a.admin, nil); rec.Code != http.StatusOK {
		t.Fatalf("version 1: %d", rec.Code)
	}
	if rec := testutil.Do(t, a.h, "PUT", "/api/v1/workflows/other", a.admin, testutil.SampleWorkflow("review")); rec.Code != http.StatusUnprocessableEntity {
		t.Fatalf("slug mismatch: %d", rec.Code)
	}
}

func TestValidateBundleEndpoint(t *testing.T) {
	a := newDefsSubsAPI(t)
	ok := map[string]any{
		"workflows": []any{testutil.SampleWorkflow("review")},
		"forms":     []any{testutil.SampleForm("contact", "review")},
	}
	rec := testutil.Do(t, a.h, "POST", "/api/v1/definitions/validate", a.admin, ok)
	if rec.Code != http.StatusOK || strings.TrimSpace(rec.Body.String()) != `{"valid":true}` {
		t.Fatalf("valid bundle: %d %s", rec.Code, rec.Body)
	}
	if rec := testutil.Do(t, a.h, "GET", "/api/v1/forms/contact", a.admin, nil); rec.Code != http.StatusNotFound {
		t.Fatalf("validate persisted the form: %d", rec.Code)
	}

	bad := map[string]any{"forms": []any{
		testutil.SampleForm("plain", ""),
		map[string]any{"slug": "Bad Slug", "title": "x", "settings": map[string]any{"public": true}, "fields": []any{}},
	}}
	rec = testutil.Do(t, a.h, "POST", "/api/v1/definitions/validate", a.admin, bad)
	if rec.Code != http.StatusUnprocessableEntity {
		t.Fatalf("invalid bundle: %d %s", rec.Code, rec.Body)
	}
	e := testutil.Decode[errResp](t, rec)
	for _, d := range e.Error.Details {
		if !strings.HasPrefix(d.Path, "forms[1]") {
			t.Fatalf("detail path %q not prefixed with forms[1]", d.Path)
		}
	}

	rec = testutil.Do(t, a.h, "POST", "/api/v1/definitions/validate", a.admin, map[string]any{"forms": []any{testutil.SampleForm("contact", "nope")}})
	if rec.Code != http.StatusUnprocessableEntity {
		t.Fatalf("dangling workflow ref: %d", rec.Code)
	}
	if rec := testutil.Do(t, a.h, "POST", "/api/v1/definitions/validate", a.env.APIKey(t, "reviewer"), ok); rec.Code != http.StatusForbidden {
		t.Fatalf("non-admin validate: %d", rec.Code)
	}
}

func TestApplyAndExportEndpoints(t *testing.T) {
	a := newDefsSubsAPI(t)
	bundle := map[string]any{
		"workflows": []any{testutil.SampleWorkflow("review")},
		"forms":     []any{testutil.SampleForm("contact", "review")},
	}
	rec := testutil.Do(t, a.h, "POST", "/api/v1/definitions/apply?dryRun=true", a.admin, bundle)
	dry := testutil.Decode[struct {
		Items []struct {
			Slug    string `json:"slug"`
			Created bool   `json:"created"`
		} `json:"items"`
	}](t, rec)
	if rec.Code != http.StatusOK || len(dry.Items) != 2 || !dry.Items[0].Created {
		t.Fatalf("dry run: %d %s", rec.Code, rec.Body)
	}
	if rec := testutil.Do(t, a.h, "GET", "/api/v1/forms/contact", a.admin, nil); rec.Code != http.StatusNotFound {
		t.Fatalf("dry run persisted: %d", rec.Code)
	}

	rec = testutil.Do(t, a.h, "POST", "/api/v1/definitions/apply?source=cli", a.admin, bundle)
	if rec.Code != http.StatusOK {
		t.Fatalf("apply: %d %s", rec.Code, rec.Body)
	}
	f, err := a.defs.GetForm(context.Background(), a.env.OrgID, "contact")
	if err != nil || f.Current.Source != "cli" {
		t.Fatalf("applied form = %+v, err = %v", f.Current, err)
	}

	rec = testutil.Do(t, a.h, "GET", "/api/v1/definitions", a.admin, nil)
	exp := testutil.Decode[struct {
		Forms     []struct{ Slug string } `json:"forms"`
		Workflows []struct{ Slug string } `json:"workflows"`
	}](t, rec)
	if len(exp.Forms) != 1 || exp.Forms[0].Slug != "contact" || len(exp.Workflows) != 1 {
		t.Fatalf("export = %+v", exp)
	}
}

func TestExportEmptyReturnsArrays(t *testing.T) {
	a := newDefsSubsAPI(t)
	rec := testutil.Do(t, a.h, "GET", "/api/v1/definitions", a.admin, nil)
	if strings.TrimSpace(rec.Body.String()) != `{"forms":[],"workflows":[]}` {
		t.Fatalf("export = %s", rec.Body)
	}
}
