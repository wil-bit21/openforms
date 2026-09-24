package httpapi_test

import (
	"net/http"
	"strings"
	"testing"

	"github.com/google/uuid"

	"github.com/openforms/openforms/internal/testutil"
)

type submitResp struct {
	ID                  string `json:"id"`
	State               string `json:"state"`
	StateLabel          string `json:"stateLabel"`
	ReceiptToken        string `json:"receiptToken"`
	ConfirmationMessage string `json:"confirmationMessage"`
}

func TestPublicGetForm(t *testing.T) {
	a := newDefsSubsAPI(t)
	a.seed(t)
	rec := testutil.Do(t, a.h, "GET", "/api/v1/public/forms/contact", "", nil)
	if rec.Code != http.StatusOK {
		t.Fatalf("public form: %d %s", rec.Code, rec.Body)
	}
	got := testutil.Decode[struct {
		Form struct {
			Slug   string `json:"slug"`
			Fields []any  `json:"fields"`
		} `json:"form"`
	}](t, rec)
	if got.Form.Slug != "contact" || len(got.Form.Fields) != 3 {
		t.Fatalf("form = %+v", got.Form)
	}
	for _, slug := range []string{"internal", "missing"} {
		rec := testutil.Do(t, a.h, "GET", "/api/v1/public/forms/"+slug, "", nil)
		if rec.Code != http.StatusNotFound || testutil.ErrorCode(t, rec) != "not_found" {
			t.Fatalf("%s: %d", slug, rec.Code)
		}
	}
}

func TestPublicSubmitAndStatus(t *testing.T) {
	a := newDefsSubsAPI(t)
	a.seed(t)
	rec := testutil.Do(t, a.h, "POST", "/api/v1/public/forms/contact/submissions", "", map[string]any{"data": validData()})
	if rec.Code != http.StatusCreated {
		t.Fatalf("submit: %d %s", rec.Code, rec.Body)
	}
	sub := testutil.Decode[submitResp](t, rec)
	if sub.State != "new" || sub.StateLabel != "New" || sub.ReceiptToken == "" || sub.ConfirmationMessage != "Thanks! Your response has been recorded." {
		t.Fatalf("submit resp = %+v", sub)
	}

	rec = testutil.Do(t, a.h, "GET", "/api/v1/public/submissions/"+sub.ID+"?token="+sub.ReceiptToken, "", nil)
	if rec.Code != http.StatusOK {
		t.Fatalf("status: %d %s", rec.Code, rec.Body)
	}
	st := testutil.Decode[struct {
		ID         string `json:"id"`
		FormTitle  string `json:"formTitle"`
		State      string `json:"state"`
		StateLabel string `json:"stateLabel"`
		Terminal   bool   `json:"terminal"`
		States     []struct {
			Key      string `json:"key"`
			Label    string `json:"label"`
			Terminal bool   `json:"terminal"`
		} `json:"states"`
		History []struct {
			State string `json:"state"`
			Label string `json:"label"`
			At    string `json:"at"`
		} `json:"history"`
		CreatedAt string `json:"createdAt"`
	}](t, rec)
	if st.ID != sub.ID || st.FormTitle != "Contact" || st.State != "new" || len(st.States) != 3 ||
		len(st.History) != 1 || st.History[0].Label != "New" || !strings.HasSuffix(st.CreatedAt, "Z") {
		t.Fatalf("status = %+v", st)
	}

	for name, path := range map[string]string{
		"wrong token":  "/api/v1/public/submissions/" + sub.ID + "?token=nope",
		"no token":     "/api/v1/public/submissions/" + sub.ID,
		"unknown id":   "/api/v1/public/submissions/" + uuid.NewString() + "?token=" + sub.ReceiptToken,
		"malformed id": "/api/v1/public/submissions/not-a-uuid?token=" + sub.ReceiptToken,
	} {
		rec := testutil.Do(t, a.h, "GET", path, "", nil)
		if rec.Code != http.StatusNotFound || testutil.ErrorCode(t, rec) != "not_found" {
			t.Fatalf("%s: %d %s", name, rec.Code, rec.Body)
		}
	}
}

func TestPublicSubmitUsesFormConfirmationMessage(t *testing.T) {
	a := newDefsSubsAPI(t)
	f := testutil.SampleForm("plain", "")
	f.Settings.ConfirmationMessage = "We'll reply within a day."
	if rec := testutil.Do(t, a.h, "PUT", "/api/v1/forms/plain", a.admin, f); rec.Code != http.StatusOK {
		t.Fatalf("PUT: %d %s", rec.Code, rec.Body)
	}
	rec := testutil.Do(t, a.h, "POST", "/api/v1/public/forms/plain/submissions", "", map[string]any{"data": validData()})
	sub := testutil.Decode[submitResp](t, rec)
	if sub.ConfirmationMessage != "We'll reply within a day." || sub.State != "submitted" {
		t.Fatalf("resp = %+v", sub)
	}
}

func TestPublicSubmitValidation(t *testing.T) {
	a := newDefsSubsAPI(t)
	a.seed(t)
	data := validData()
	delete(data, "email")
	rec := testutil.Do(t, a.h, "POST", "/api/v1/public/forms/contact/submissions", "", map[string]any{"data": data})
	if rec.Code != http.StatusUnprocessableEntity {
		t.Fatalf("status = %d %s", rec.Code, rec.Body)
	}
	e := testutil.Decode[errResp](t, rec)
	found := false
	for _, d := range e.Error.Details {
		if d.Path == "data.email" {
			found = true
		}
	}
	if !found {
		t.Fatalf("details = %+v", e.Error.Details)
	}
	rec = testutil.Do(t, a.h, "POST", "/api/v1/public/forms/contact/submissions", "", map[string]any{})
	if rec.Code != http.StatusUnprocessableEntity {
		t.Fatalf("missing data object: %d", rec.Code)
	}
}

func TestPublicSubmitPrivateOrUnknownForm(t *testing.T) {
	a := newDefsSubsAPI(t)
	a.seed(t)
	for _, slug := range []string{"internal", "missing"} {
		rec := testutil.Do(t, a.h, "POST", "/api/v1/public/forms/"+slug+"/submissions", "", map[string]any{"data": validData()})
		if rec.Code != http.StatusNotFound {
			t.Fatalf("%s: %d", slug, rec.Code)
		}
	}
}

func TestPublicSubmitBadBodies(t *testing.T) {
	a := newDefsSubsAPI(t)
	a.seed(t)
	rec := testutil.Do(t, a.h, "POST", "/api/v1/public/forms/contact/submissions", "", "not an object")
	if rec.Code != http.StatusBadRequest || testutil.ErrorCode(t, rec) != "bad_request" {
		t.Fatalf("string body: %d %s", rec.Code, rec.Body)
	}
	huge := validData()
	huge["name"] = strings.Repeat("a", 2<<20)
	rec = testutil.Do(t, a.h, "POST", "/api/v1/public/forms/contact/submissions", "", map[string]any{"data": huge})
	if rec.Code != http.StatusRequestEntityTooLarge || testutil.ErrorCode(t, rec) != "bad_request" {
		t.Fatalf("2 MiB body: %d", rec.Code)
	}
}

func TestPublicSubmitRateLimited(t *testing.T) {
	a := newDefsSubsAPI(t)
	a.seed(t)
	for i := 0; i < 20; i++ {
		rec := testutil.Do(t, a.h, "POST", "/api/v1/public/forms/plain/submissions", "", map[string]any{"data": validData()})
		if rec.Code != http.StatusCreated {
			t.Fatalf("submission %d: %d %s", i+1, rec.Code, rec.Body)
		}
	}
	rec := testutil.Do(t, a.h, "POST", "/api/v1/public/forms/plain/submissions", "", map[string]any{"data": validData()})
	if rec.Code != http.StatusTooManyRequests || testutil.ErrorCode(t, rec) != "rate_limited" || rec.Header().Get("Retry-After") != "60" {
		t.Fatalf("21st submission: %d %s", rec.Code, rec.Body)
	}
}
