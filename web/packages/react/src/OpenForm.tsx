import type { Field, FormDefinition, OpenFormsClient, PublicSubmitResult } from "@openforms/sdk";
import { useEffect, useId, useRef, useState, type FormEvent, type ReactNode } from "react";
import { defaultComponents, TextInput, type FieldComponents } from "./fields.js";
import { useOpenForm, type SubmitHandler } from "./useOpenForm.js";

export interface OpenFormProps {
  client?: OpenFormsClient;
  slug?: string;
  definition?: FormDefinition;
  onSubmit?: SubmitHandler;
  onSubmitted?: (result: PublicSubmitResult | undefined) => void;
  onLoaded?: (form: FormDefinition) => void;
  components?: FieldComponents;
  className?: string;
  hideHeader?: boolean;
  renderConfirmation?: (result: PublicSubmitResult | undefined, form: FormDefinition) => ReactNode;
  renderLoadError?: (error: Error) => ReactNode;
}

const FORM_KEY = "_form";
const DEFAULT_CONFIRMATION = "Thanks! Your response has been recorded.";

const cx = (...names: Array<string | false | null | undefined>) => names.filter(Boolean).join(" ");

export function OpenForm(props: OpenFormProps) {
  const { client, slug, definition, onSubmit, components, className, hideHeader } = props;
  if (!definition && !(client && slug)) {
    throw new Error("<OpenForm> needs either `definition` or both `client` and `slug`.");
  }
  const state = useOpenForm(definition ? { definition, onSubmit } : { client: client!, slug: slug! });
  const { form, status, values, setValue, visible, errors, result, loadError } = state;

  const idBase = `of${useId().replace(/:/g, "")}-`;
  const fieldId = (key: string) => `${idBase}${key}`;
  const titleId = `${idBase}title`;

  const summaryRef = useRef<HTMLDivElement>(null);
  const [failedAttempts, setFailedAttempts] = useState(0);
  useEffect(() => {
    if (failedAttempts > 0) summaryRef.current?.focus();
  }, [failedAttempts]);

  const onLoadedRef = useRef(props.onLoaded);
  onLoadedRef.current = props.onLoaded;
  useEffect(() => {
    if (form) onLoadedRef.current?.(form);
  }, [form]);

  const notified = useRef(false);
  const onSubmittedRef = useRef(props.onSubmitted);
  onSubmittedRef.current = props.onSubmitted;
  useEffect(() => {
    if (status === "submitted" && !notified.current) {
      notified.current = true;
      onSubmittedRef.current?.(result);
    }
    if (status !== "submitted") notified.current = false;
  }, [status, result]);

  if (status === "error") {
    return (
      <div className={cx("of-form", className)} role="alert">
        {props.renderLoadError && loadError ? (
          props.renderLoadError(loadError)
        ) : (
          <p className="of-load-error">This form isn't available right now.</p>
        )}
      </div>
    );
  }

  if (!form) {
    return (
      <div className={cx("of-form", "of-loading", className)} aria-busy="true">
        Loading form…
      </div>
    );
  }

  if (status === "submitted") {
    return (
      <div className={cx("of-form", className)}>
        {props.renderConfirmation ? (
          props.renderConfirmation(result, form)
        ) : (
          <div className="of-confirmation" role="status">
            <p>{result?.confirmationMessage || form.settings?.confirmationMessage || DEFAULT_CONFIRMATION}</p>
          </div>
        )}
      </div>
    );
  }

  const fields: Field[] = form.fields ?? [];
  const labelFor = (key: string) => fields.find((f) => f.key === key)?.label ?? key;
  const fieldErrors = Object.entries(errors).filter(([key]) => key !== FORM_KEY);
  const formError = errors[FORM_KEY];
  const submitting = status === "submitting";

  async function handleSubmit(event: FormEvent<HTMLFormElement>) {
    event.preventDefault();
    const ok = await state.submit();
    if (!ok) setFailedAttempts((n) => n + 1);
  }

  function renderField(field: Field) {
    const id = fieldId(field.key);
    const error = errors[field.key];
    const helpId = field.help ? `${id}-help` : undefined;
    const errorId = error ? `${id}-error` : undefined;
    const describedBy = [helpId, errorId].filter(Boolean).join(" ") || undefined;
    const Component = components?.[field.type] ?? defaultComponents[field.type] ?? TextInput;
    const required = Boolean(field.required);

    const input = (
      <Component
        field={field}
        id={id}
        value={values[field.key]}
        onChange={(v) => setValue(field.key, v)}
        invalid={Boolean(error)}
        describedBy={describedBy}
        required={required}
        disabled={submitting}
      />
    );
    const marker = required ? (
      <span className="of-required" aria-hidden="true">
        {" *"}
      </span>
    ) : null;
    const help = field.help ? (
      <p id={helpId} className="of-help">
        {field.help}
      </p>
    ) : null;
    const errorText = error ? (
      <p id={errorId} className="of-field-error">
        {error}
      </p>
    ) : null;
    const wrapperClass = cx("of-field", `of-field-${field.type}`, error && "of-field-invalid");

    if (field.type === "multiselect") {
      return (
        <fieldset key={field.key} className={wrapperClass} aria-describedby={describedBy}>
          <legend className="of-label">
            {field.label}
            {marker}
          </legend>
          {help}
          {input}
          {errorText}
        </fieldset>
      );
    }
    if (field.type === "checkbox") {
      return (
        <div key={field.key} className={wrapperClass}>
          <div className="of-checkbox-row">
            {input}
            <label htmlFor={id} className="of-label">
              {field.label}
              {marker}
            </label>
          </div>
          {help}
          {errorText}
        </div>
      );
    }
    return (
      <div key={field.key} className={wrapperClass}>
        <label htmlFor={id} className="of-label">
          {field.label}
          {marker}
        </label>
        {help}
        {input}
        {errorText}
      </div>
    );
  }

  return (
    <form
      className={cx("of-form", className)}
      noValidate
      onSubmit={handleSubmit}
      aria-labelledby={hideHeader ? undefined : titleId}
      aria-busy={submitting || undefined}
    >
      {!hideHeader && (
        <header className="of-header">
          <h2 id={titleId} className="of-title">
            {form.title}
          </h2>
          {form.description && <p className="of-description">{form.description}</p>}
        </header>
      )}

      {(fieldErrors.length > 0 || formError) && (
        <div ref={summaryRef} className="of-error-summary" role="alert" tabIndex={-1}>
          {fieldErrors.length > 0 ? (
            <>
              <p className="of-error-summary-title">
                Please fix {fieldErrors.length} {fieldErrors.length === 1 ? "field" : "fields"} before submitting.
              </p>
              <ul>
                {fieldErrors.map(([key, message]) => (
                  <li key={key}>
                    <a
                      href={`#${fieldId(key)}`}
                      onClick={(e) => {
                        e.preventDefault();
                        document.getElementById(fieldId(key))?.focus();
                      }}
                    >
                      {labelFor(key)}: {message}
                    </a>
                  </li>
                ))}
              </ul>
              {formError && <p>{formError}</p>}
            </>
          ) : (
            <p className="of-error-summary-title">{formError}</p>
          )}
        </div>
      )}

      {fields.filter((f) => visible[f.key]).map(renderField)}

      <div className="of-actions">
        <button type="submit" className="of-submit" disabled={submitting}>
          {submitting ? "Sending…" : form.settings?.submitLabel || "Submit"}
        </button>
      </div>
    </form>
  );
}
