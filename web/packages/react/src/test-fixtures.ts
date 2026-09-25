import { OpenFormsClient, type FormDefinition, type PublicSubmitResult } from "@openforms/sdk";

export const API = "http://api.test";

export const newClient = () => new OpenFormsClient({ baseUrl: API });

export const jobForm: FormDefinition = {
  slug: "job-application",
  title: "Job application",
  description: "Apply to join the team.",
  workflow: "hiring",
  settings: { public: true, submitLabel: "Send application", confirmationMessage: "Thanks! We'll be in touch." },
  fields: [
    { key: "name", type: "text", label: "Full name", required: true, validation: { minLength: 2, maxLength: 100 } },
    { key: "email", type: "email", label: "Email", required: true, help: "We only use this to reply to you." },
    {
      key: "role",
      type: "select",
      label: "Role",
      required: true,
      options: [
        { value: "engineer", label: "Engineer" },
        { value: "designer", label: "Designer" },
      ],
    },
    { key: "portfolio", type: "url", label: "Portfolio URL", required: true, showIf: { field: "role", equals: "designer" } },
    { key: "years", type: "number", label: "Years of experience", validation: { min: 0, max: 50 } },
    {
      key: "skills",
      type: "multiselect",
      label: "Skills",
      options: [
        { value: "go", label: "Go" },
        { value: "ts", label: "TypeScript" },
      ],
    },
    { key: "start", type: "date", label: "Earliest start date" },
    { key: "cover", type: "textarea", label: "Cover letter", placeholder: "Tell us about yourself" },
    { key: "consent", type: "checkbox", label: "I agree to the privacy policy", required: true },
  ],
};

export const submitResult: PublicSubmitResult = {
  id: "sub-1",
  state: "new",
  stateLabel: "New",
  receiptToken: "tok-123",
  confirmationMessage: "Thanks! We'll be in touch.",
};
