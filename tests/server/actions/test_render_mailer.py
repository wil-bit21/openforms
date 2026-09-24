import datetime as dt
import uuid

import pytest

from openforms.definition import Form, Transition
from openforms.server.actions import render, template_vars
from openforms.server.actions.mailer import LogMailer, SMTPMailer, build_message, new_mailer
from openforms.server.models.submissions import Submission
from tests.conftest import test_settings

VARS = {
    "submission": {
        "data": {
            "name": "Ada",
            "score": 3.0,
            "ratio": 0.25,
            "ok": True,
            "tags": ["a", "b"],
            "nested": {"x": 1.0},
            "none": None,
        }
    },
    "baseUrl": "https://forms.example.com",
}


@pytest.mark.parametrize(
    "tmpl,want",
    [
        ("Hi {{submission.data.name}}!", "Hi Ada!"),
        ("Hi {{ submission.data.name }}!", "Hi Ada!"),
        ("{{submission.data.score}}", "3"),
        ("{{submission.data.ratio}}", "0.25"),
        ("{{submission.data.ok}}", "true"),
        ("{{submission.data.tags}}", "a, b"),
        ("{{submission.data.nested}}", '{"x":1}'),
        ("[{{submission.data.none}}]", "[]"),
        ("[{{submission.data.missing}}]", "[]"),
        ("[{{nope.deeper.still}}]", "[]"),
        ("[{{submission.data.name.first}}]", "[]"),
        ("{{baseUrl}}/x", "https://forms.example.com/x"),
        ("no placeholders", "no placeholders"),
        ("{{ not closed", "{{ not closed"),
        ("<b>{{submission.data.name}}</b>", "<b>Ada</b>"),
    ],
)
def test_render(tmpl, want):
    assert render(tmpl, VARS) == want


def test_template_vars():
    sid = uuid.UUID("11111111-2222-3333-4444-555555555555")
    sub = Submission(
        id=sid,
        org_id=sid,
        form_id=sid,
        form_version_id=sid,
        form_slug="apply",
        form_version=1,
        workflow_version_id=None,
        state="screening",
        state_label="Screening",
        data={"email": "ada@example.com"},
        fields={"score": 4.0},
    )
    vars_ = template_vars(
        test_settings(base_url="https://forms.example.com/"),
        Form(slug="apply", title="Apply"),
        sub,
        Transition(key="invite", label="Invite"),
    )
    checks = {
        "{{submission.id}}": str(sid),
        "{{submission.state}}": "screening",
        "{{submission.stateLabel}}": "Screening",
        "{{submission.data.email}}": "ada@example.com",
        "{{submission.fields.score}}": "4",
        "{{submission.url}}": f"https://forms.example.com/admin/submissions/{sid}",
        "{{form.slug}}": "apply",
        "{{form.title}}": "Apply",
        "{{transition.key}}": "invite",
        "{{transition.label}}": "Invite",
        "{{baseUrl}}": "https://forms.example.com",
        "[{{submission.statusUrl}}]": "[]",
    }
    for tmpl, want in checks.items():
        assert render(tmpl, vars_) == want, tmpl
    sub.data = {}
    no_tr = template_vars(test_settings(), Form(), sub, None)
    assert render("[{{transition.key}}][{{submission.data.x}}]", no_tr) == "[][]"


async def test_new_mailer():
    assert isinstance(new_mailer(test_settings()), LogMailer)
    m = new_mailer(test_settings(smtp_host="mail.local", smtp_port=1025, smtp_from="of@x.test"))
    assert isinstance(m, SMTPMailer) and m.addr == "mail.local:1025" and m.from_ == "of@x.test"
    await LogMailer().send("a@b.c", "s", "b")


def test_build_message():
    msg = build_message(
        "of@x.test", "ada@example.com", "Hello Ada", "line1\nline2", dt.datetime(2030, 1, 2, 3, 4, 5, tzinfo=dt.UTC)
    ).decode()
    for want in (
        "From: of@x.test\r\n",
        "To: ada@example.com\r\n",
        "Subject: Hello Ada\r\n",
        "Date: Wed, 02 Jan 2030 03:04:05 +0000\r\n",
        "MIME-Version: 1.0\r\n",
        "Content-Type: text/plain; charset=utf-8\r\n",
        "\r\n\r\nline1\r\nline2",
    ):
        assert want in msg


def test_build_message_no_header_injection():
    msg = build_message(
        "of@x.test", "ada@example.com", "Hi\r\nBcc: evil@x.test", "body", dt.datetime.now(dt.UTC)
    ).decode()
    assert not any(line.startswith("Bcc:") for line in msg.split("\r\n"))
    with pytest.raises(ValueError):
        build_message("of@x.test", "ada@example.com\r\nBcc: evil@x.test", "s", "b", dt.datetime.now(dt.UTC))
    with pytest.raises(ValueError):
        build_message("of@x.test", "not an address", "s", "b", dt.datetime.now(dt.UTC))


def test_build_message_encodes_non_ascii_subject():
    msg = build_message("of@x.test", "ada@example.com", "Grüße", "b", dt.datetime.now(dt.UTC)).decode()
    assert "Subject: =?utf-8?q?Gr=C3=BC=C3=9Fe?=\r\n" in msg


def test_long_subject_is_split_into_encoded_words():
    from openforms.server.actions.mailer import q_encode

    enc = q_encode("ü" * 40)
    assert all(len(w) <= 75 for w in enc.split(" ")) and enc.count("=?utf-8?q?") > 1


async def test_smtp_mailer_send():
    got = {}

    def fake(host, port, auth, from_, to, msg):
        got.update(host=host, port=port, auth=auth, from_=from_, to=to, msg=msg)

    m = SMTPMailer(host="mail.local", port=1025, from_="of@x.test", username="u", password="p", send_func=fake)
    await m.send("ada@example.com", "Subj", "Body")
    assert (got["host"], got["port"], got["from_"], got["to"]) == ("mail.local", 1025, "of@x.test", ["ada@example.com"])
    assert got["auth"] == ("u", "p") and b"Subject: Subj" in got["msg"]
    m.username = ""
    await m.send("ada@example.com", "s", "b")
    assert got["auth"] is None
