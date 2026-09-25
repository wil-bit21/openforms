import {
  FORM_ERROR_KEY,
  OpenFormsError,
  problemsToErrors,
  validateSubmission,
  visible as computeVisible,
  type FormDefinition,
  type OpenFormsClient,
  type PublicSubmitResult,
  type Values,
} from "@openforms/sdk";
import { useCallback, useEffect, useMemo, useRef, useState } from "react";

export type OpenFormStatus = "loading" | "ready" | "submitting" | "submitted" | "error";

export type SubmitHandler = (data: Values) => Promise<PublicSubmitResult | void> | PublicSubmitResult | void;

export type UseOpenFormOptions =
  | { client: OpenFormsClient; slug: string; definition?: undefined; onSubmit?: undefined }
  | { definition: FormDefinition; onSubmit?: SubmitHandler; client?: undefined; slug?: undefined };

export interface UseOpenFormResult {
  form: FormDefinition | null;
  status: OpenFormStatus;
  values: Values;
  setValue(key: string, value: unknown): void;
  visible: Record<string, boolean>;
  errors: Record<string, string>;
  submit(): Promise<boolean>;
  result: PublicSubmitResult | undefined;
  loadError: Error | null;
  reset(): void;
}

const GENERIC_FAILURE = "Something went wrong while sending your response. Please try again.";
const RATE_LIMITED = "Too many submissions from your network. Please wait a minute and try again.";

export function useOpenForm(options: UseOpenFormOptions): UseOpenFormResult {
  const { client, slug, definition } = options;
  const onSubmitRef = useRef(options.onSubmit);
  onSubmitRef.current = options.onSubmit;

  const [loaded, setLoaded] = useState<FormDefinition | null>(null);
  const [loadError, setLoadError] = useState<Error | null>(null);
  const [values, setValues] = useState<Values>({});
  const [errors, setErrors] = useState<Record<string, string>>({});
  const [phase, setPhase] = useState<"idle" | "submitting" | "submitted">("idle");
  const [result, setResult] = useState<PublicSubmitResult | undefined>(undefined);
  const inFlight = useRef(false);

  useEffect(() => {
    if (definition || !client || !slug) return;
    let cancelled = false;
    setLoaded(null);
    setLoadError(null);
    client.getPublicForm(slug).then(
      (form) => {
        if (!cancelled) setLoaded(form);
      },
      (err: unknown) => {
        if (!cancelled) setLoadError(err instanceof Error ? err : new Error(String(err)));
      },
    );
    return () => {
      cancelled = true;
    };
  }, [client, slug, definition]);

  const form = definition ?? loaded;
  const visibleMap = useMemo(() => (form ? computeVisible(form, values) : {}), [form, values]);
  const status: OpenFormStatus = loadError ? "error" : !form ? "loading" : phase === "idle" ? "ready" : phase;

  const setValue = useCallback((key: string, value: unknown) => {
    setValues((prev) => ({ ...prev, [key]: value }));
    setErrors((prev) => {
      if (!(key in prev) && !(FORM_ERROR_KEY in prev)) return prev;
      const next = { ...prev };
      delete next[key];
      delete next[FORM_ERROR_KEY];
      return next;
    });
  }, []);

  const submit = useCallback(async (): Promise<boolean> => {
    if (!form || inFlight.current) return false;
    const { clean, problems } = validateSubmission(form, values);
    if (!clean) {
      setErrors(problemsToErrors(problems));
      return false;
    }

    inFlight.current = true;
    setPhase("submitting");
    setErrors({});
    try {
      let res: PublicSubmitResult | undefined;
      if (definition) {
        res = (await onSubmitRef.current?.(clean)) ?? undefined;
      } else if (client && slug) {
        res = await client.submitPublic(slug, clean);
      }
      setResult(res);
      setPhase("submitted");
      return true;
    } catch (err) {
      setPhase("idle");
      if (err instanceof OpenFormsError && err.code === "validation_failed" && err.details.length > 0) {
        setErrors(problemsToErrors(err.details));
      } else if (err instanceof OpenFormsError && err.code === "rate_limited") {
        setErrors({ [FORM_ERROR_KEY]: RATE_LIMITED });
      } else {
        setErrors({ [FORM_ERROR_KEY]: GENERIC_FAILURE });
      }
      return false;
    } finally {
      inFlight.current = false;
    }
  }, [form, values, definition, client, slug]);

  const reset = useCallback(() => {
    setValues({});
    setErrors({});
    setResult(undefined);
    setPhase("idle");
  }, []);

  return { form, status, values, setValue, visible: visibleMap, errors, submit, result, loadError, reset };
}
