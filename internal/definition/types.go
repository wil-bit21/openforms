package definition

// FieldType is the type of a form field or workflow field.
type FieldType string

const (
	FieldText        FieldType = "text"
	FieldTextarea    FieldType = "textarea"
	FieldEmail       FieldType = "email"
	FieldNumber      FieldType = "number"
	FieldSelect      FieldType = "select"
	FieldMultiselect FieldType = "multiselect"
	FieldCheckbox    FieldType = "checkbox"
	FieldDate        FieldType = "date"
	FieldURL         FieldType = "url"
)

type Option struct {
	Value string `json:"value"`
	Label string `json:"label"`
}

type Validation struct {
	MinLength *int     `json:"minLength,omitempty"`
	MaxLength *int     `json:"maxLength,omitempty"`
	Pattern   string   `json:"pattern,omitempty"`
	Min       *float64 `json:"min,omitempty"`
	Max       *float64 `json:"max,omitempty"`
}

// Condition controls field visibility. Exactly one of Equals, NotEquals, In is set.
type Condition struct {
	Field     string `json:"field"`
	Equals    any    `json:"equals,omitempty"`
	NotEquals any    `json:"notEquals,omitempty"`
	In        []any  `json:"in,omitempty"`
}

type Field struct {
	Key         string      `json:"key"`
	Type        FieldType   `json:"type"`
	Label       string      `json:"label"`
	Help        string      `json:"help,omitempty"`
	Placeholder string      `json:"placeholder,omitempty"`
	Required    bool        `json:"required,omitempty"`
	Options     []Option    `json:"options,omitempty"`
	Validation  *Validation `json:"validation,omitempty"`
	ShowIf      *Condition  `json:"showIf,omitempty"`
}

type FormSettings struct {
	Public              bool   `json:"public"`
	SubmitLabel         string `json:"submitLabel,omitempty"`
	ConfirmationMessage string `json:"confirmationMessage,omitempty"`
}

type Form struct {
	Slug        string       `json:"slug"`
	Title       string       `json:"title"`
	Description string       `json:"description,omitempty"`
	Workflow    string       `json:"workflow,omitempty"`
	Settings    FormSettings `json:"settings"`
	Fields      []Field      `json:"fields"`
}

type State struct {
	Key      string `json:"key"`
	Label    string `json:"label"`
	Color    string `json:"color,omitempty"`
	Terminal bool   `json:"terminal,omitempty"`
}

type WorkflowField struct {
	Key     string    `json:"key"`
	Type    FieldType `json:"type"`
	Label   string    `json:"label"`
	Options []Option  `json:"options,omitempty"`
}

type Guard struct {
	Roles         []string `json:"roles,omitempty"`
	RequireFields []string `json:"requireFields,omitempty"`
}

// ActionType is the kind of side effect a transition or submission triggers.
type ActionType string

const (
	ActionWebhook ActionType = "webhook"
	ActionEmail   ActionType = "email"
	ActionAssign  ActionType = "assign"
)

type Action struct {
	Type    ActionType `json:"type"`
	URL     string     `json:"url,omitempty"`
	To      string     `json:"to,omitempty"`
	Subject string     `json:"subject,omitempty"`
	Body    string     `json:"body,omitempty"`
	User    string     `json:"user,omitempty"`
	Role    string     `json:"role,omitempty"`
}

type Transition struct {
	Key     string   `json:"key"`
	Label   string   `json:"label"`
	From    []string `json:"from"`
	To      string   `json:"to"`
	Guard   Guard    `json:"guard"`
	Actions []Action `json:"actions,omitempty"`
}

type Workflow struct {
	Slug        string          `json:"slug"`
	Title       string          `json:"title"`
	Initial     string          `json:"initial"`
	States      []State         `json:"states"`
	Fields      []WorkflowField `json:"fields,omitempty"`
	OnSubmit    []Action        `json:"onSubmit,omitempty"`
	Transitions []Transition    `json:"transitions"`
}

// State returns the state with the given key.
func (w Workflow) State(key string) (State, bool) {
	for _, s := range w.States {
		if s.Key == key {
			return s, true
		}
	}
	return State{}, false
}

// Transition returns the transition with the given key.
func (w Workflow) Transition(key string) (Transition, bool) {
	for _, t := range w.Transitions {
		if t.Key == key {
			return t, true
		}
	}
	return Transition{}, false
}

// Field returns the workflow field with the given key.
func (w Workflow) Field(key string) (WorkflowField, bool) {
	for _, f := range w.Fields {
		if f.Key == key {
			return f, true
		}
	}
	return WorkflowField{}, false
}
