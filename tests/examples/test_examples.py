"""Port of ``examples/examples_test.go``."""

from openforms._paths import examples_dir
from openforms.definition import parse_form, parse_workflow
from openforms.definition.bundle import validate_bundle
from openforms.examples import bundle


def form(slug):
    return next(f for f in bundle()[0] if f.slug == slug)


def workflow(slug):
    return next(w for w in bundle()[1] if w.slug == slug)


def test_bundle_is_valid():
    forms, workflows = bundle()
    validate_bundle(forms, workflows)
    assert [f.slug for f in forms] == ["contact", "job-application"]
    assert [w.slug for w in workflows] == ["contact-triage", "hiring"]


def test_slugs_match_file_names():
    for sub, parse in (("forms", parse_form), ("workflows", parse_workflow)):
        for p in (examples_dir() / sub).iterdir():
            assert parse(p.read_bytes()).slug == p.stem, p


def test_job_application_shape():
    f = form("job-application")
    assert [x.key for x in f.fields] == ["name", "email", "role", "years", "portfolio", "coverLetter", "consent"]
    assert f.workflow == "hiring" and f.settings.public
    show_if = f.fields[4].show_if
    assert show_if is not None and (show_if.field, show_if.equals) == ("role", "designer")
    assert f.fields[6].type == "checkbox" and f.fields[6].required


def test_hiring_workflow_shape():
    w = workflow("hiring")
    assert [s.key for s in w.states] == ["new", "screening", "interview", "hired", "rejected"]
    trs = {t.key: t for t in w.transitions}
    assert list(trs) == ["screen", "invite", "hire", "reject"]
    assert trs["reject"].guard.require_fields == ["rejectionReason"]
    assert trs["hire"].guard.roles == ["hiring-manager"]
    assert len(w.on_submit) == 2
    for t in w.transitions:
        assert all(a.type != "webhook" for a in t.actions or []), t.key


def test_contact_triage_shape():
    assert sorted(s.key for s in workflow("contact-triage").states) == ["in_progress", "open", "resolved", "spam"]
    assert form("contact").workflow == "contact-triage"
