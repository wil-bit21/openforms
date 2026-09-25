"""Shared sample definitions (port of Go ``testutil.SampleWorkflow`` / ``SampleForm``)."""

from openforms.definition import Field, Form, FormSettings, Guard, Option, State, Transition, Workflow, WorkflowField


def sample_workflow(slug: str) -> Workflow:
    """new → review → done. "start" requires the reviewer role; "finish" requires the "note" field."""
    return Workflow(
        slug=slug,
        title="Review",
        initial="new",
        states=[
            State("new", "New", "gray"),
            State("review", "In review", "blue"),
            State("done", "Done", "green", terminal=True),
        ],
        fields=[WorkflowField("note", "textarea", "Note")],
        transitions=[
            Transition("start", "Start review", ["new"], "review", Guard(roles=["reviewer"])),
            Transition("finish", "Finish", ["review"], "done", Guard(require_fields=["note"])),
        ],
    )


def sample_form(slug: str, workflow: str = "") -> Form:
    """A valid public contact form."""
    return Form(
        slug=slug,
        title="Contact",
        workflow=workflow,
        settings=FormSettings(public=True),
        fields=[
            Field(key="name", type="text", label="Name", required=True),
            Field(key="email", type="email", label="Email", required=True),
            Field(
                key="topic",
                type="select",
                label="Topic",
                options=[Option("sales", "Sales"), Option("support", "Support")],
            ),
        ],
    )
