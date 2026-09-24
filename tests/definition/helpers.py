from openforms.definition import (
    Action,
    Condition,
    Field,
    Form,
    FormSettings,
    Guard,
    Option,
    State,
    Transition,
    Validation,
    ValidationError,
    Workflow,
    WorkflowField,
)


def problem_paths(fn, *args) -> list[str]:
    try:
        fn(*args)
    except ValidationError as e:
        return sorted(p.path for p in e.problems)
    return []


def base_form() -> Form:
    return Form(
        slug="job-application",
        title="Job application",
        workflow="hiring",
        settings=FormSettings(public=True),
        fields=[
            Field(
                key="name",
                type="text",
                label="Full name",
                required=True,
                validation=Validation(min_length=2, max_length=100),
            ),
            Field(
                key="role",
                type="select",
                label="Role",
                required=True,
                options=[Option("engineer", "Engineer"), Option("designer", "Designer")],
            ),
            Field(key="portfolio", type="url", label="Portfolio", show_if=Condition(field="role", equals="designer")),
            Field(key="years", type="number", label="Years of experience", validation=Validation(min=0, max=50)),
        ],
    )


def base_workflow() -> Workflow:
    return Workflow(
        slug="hiring",
        title="Hiring pipeline",
        initial="new",
        states=[
            State("new", "New", "gray"),
            State("screening", "Screening", "blue"),
            State("interview", "Interview", "purple"),
            State("hired", "Hired", "green", terminal=True),
            State("rejected", "Rejected", "red", terminal=True),
        ],
        fields=[
            WorkflowField("score", "number", "Score"),
            WorkflowField("rejectionReason", "textarea", "Rejection reason"),
            WorkflowField("source", "select", "Source", [Option("referral", "Referral"), Option("website", "Website")]),
        ],
        on_submit=[
            Action(type="assign", role="reviewer"),
            Action(
                type="email",
                to="{{submission.data.email}}",
                subject="We got your application",
                body="Hi {{submission.data.name}}",
            ),
        ],
        transitions=[
            Transition("screen", "Start screening", ["new"], "screening", Guard(roles=["reviewer"])),
            Transition(
                "invite",
                "Invite to interview",
                ["screening"],
                "interview",
                Guard(roles=["reviewer"], require_fields=["score"]),
                [Action(type="webhook", url="https://example.com/hooks/interview")],
            ),
            Transition("hire", "Hire", ["interview"], "hired", Guard(roles=["hiring-manager"])),
            Transition(
                "reject",
                "Reject",
                ["new", "screening", "interview"],
                "rejected",
                Guard(roles=["reviewer", "hiring-manager"], require_fields=["rejectionReason"]),
                [
                    Action(
                        type="email",
                        to="{{submission.data.email}}",
                        subject="Your application",
                        body="{{submission.fields.rejectionReason}}",
                    )
                ],
            ),
        ],
    )
