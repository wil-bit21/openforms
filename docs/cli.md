# CLI reference

`openforms` is one command: it runs the server and it manages definitions. It is not published to PyPI; install it from the repository (Python ≥ 3.11) or use the Docker image:

```bash
uv tool install git+https://github.com/openforms/openforms
# from a checkout:
uv run openforms --help
# or, with Docker (mount your project to use push/pull/diff):
docker run --rm -v "$PWD:/work" -w /work -e OPENFORMS_URL -e OPENFORMS_API_KEY openforms:dev push
```

## Connecting to a server

Remote commands (`push`, `pull`, `diff`) resolve settings in this order:

1. flags `--server` and `--api-key`
2. environment `OPENFORMS_URL` and `OPENFORMS_API_KEY`
3. `openforms.yaml` in the current directory:

```yaml
server: http://localhost:8080
dir: openforms
```

Server commands (`serve`, `migrate`, `seed`, `admin …`) connect to the database directly using `OPENFORMS_DATABASE_URL` and the other variables in [self-hosting](self-hosting.md).

## Definitions layout

```
openforms/
  forms/<slug>.yaml
  workflows/<slug>.yaml
```

`.yaml`, `.yml` and `.json` files are read. A file's `slug` must equal its file name.

## Commands

### `openforms init [dir]`

Creates `openforms.yaml`, `openforms/forms/contact.yaml` and `openforms/workflows/contact-triage.yaml` in `dir` (default `.`). It never overwrites existing files.

### `openforms validate [--dir openforms]`

Checks every definition offline, including references between forms and workflows. Problems are printed as `file:path: message`, for example:

```
openforms/forms/contact.yaml:fields[2].showIf.field: must reference an earlier field
```

Exit code 0 when valid, 1 otherwise.

### `openforms push [--dir openforms] [--dry-run]`

Validates locally, then applies all definitions atomically (`POST /api/v1/definitions/apply?source=cli`):

```
KIND      SLUG             VERSION  STATUS
workflow  contact-triage   3        updated
form      contact          4        updated
form      newsletter       1        created
```

Unchanged files report `unchanged` and create no versions. `--dry-run` shows the same table without saving anything.

### `openforms pull [--dir openforms] [--check]`

Downloads every definition and writes canonical YAML files. `--check` writes nothing and exits 1 if any file would change, which makes it the CI drift check.

### `openforms diff [--dir openforms]`

Compares local files with the server:

```
~ form contact            differs
+ form newsletter         local only
- workflow old-triage     remote only
```

### `openforms serve`

Runs migrations and starts the HTTP server and background worker. With `OPENFORMS_SEED_DEMO=true` it also seeds the demo bundle.

### `openforms migrate`

Runs database migrations and exits.

### `openforms seed --demo`

Loads the example forms and workflows (source `seed`) and creates the demo users `admin@demo.local`, `reviewer@demo.local` and `manager@demo.local` (password `demo1234`) if they don't exist. Safe to run repeatedly.

### `openforms admin create-user --email E --name N --password P --roles r1,r2`

Creates a user. Use this to create your first admin (`--roles admin`).

### `openforms admin create-api-key --name N --roles r1,r2`

Creates an API key and prints it once.

## CI recipe (GitHub Actions)

```yaml
- run: openforms validate
- run: openforms pull --check
  env:
    OPENFORMS_URL: ${{ secrets.OPENFORMS_URL }}
    OPENFORMS_API_KEY: ${{ secrets.OPENFORMS_API_KEY }}
- if: github.ref == 'refs/heads/main'
  run: openforms push
  env:
    OPENFORMS_URL: ${{ secrets.OPENFORMS_URL }}
    OPENFORMS_API_KEY: ${{ secrets.OPENFORMS_API_KEY }}
```
