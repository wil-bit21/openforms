# Conformance fixtures

Shared by the Python server (`openforms.definition`) and the TypeScript SDK
(`@openforms/sdk`). Both test suites iterate **every** case; the server is
authoritative, and TS must agree on every case.

- `visibility.json`: `[{ name, form, data, visible }]`. `visible` lists every field key.
- `submission.json`: `[{ name, form, data, clean, errorPaths }]`. When `clean` is
  non-null the submission is valid and the cleaned data must deep-equal it.
  Otherwise the sorted problem paths must equal the sorted `errorPaths`
  (comparison is order-insensitive).

Rules the fixtures encode (spec §5.4 plus Plan 02 clarifications):

1. String answers are trimmed; blank after trimming = not provided.
2. `null`, blank strings and empty arrays are "not provided"; `false` is a provided checkbox value.
3. Lengths count Unicode code points (`[...s].length` in JS).
4. `pattern` is anchored: the whole value must match `^(?:pattern)$`.
5. `email` is a bare address `local@domain` (no display name, no angle brackets, no spaces).
6. `url` is absolute `http`/`https` with a host. `date` is a real calendar date in `YYYY-MM-DD`.
7. Visibility uses the *cleaned* controller value; invalid = absent. `equals`/`in` on
   absent → hidden; `notEquals` on absent → visible; a hidden controller hides its
   dependants. For `multiselect` controllers: `equals` = contains, `notEquals` = does
   not contain, `in` = contains any.
8. Hidden fields and unknown keys are dropped; hidden fields are never validated.
