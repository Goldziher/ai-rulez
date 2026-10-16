# Parcel tracker

Python 3.12 API (FastAPI) and a React admin app.

## Commands

- `make test` runs the Python tests; `pnpm --dir admin test` runs the admin tests.
- `make lint` runs ruff and mypy; CI fails on either.
- `make dev` starts the API on port 8000 with the SQLite file `dev.db`.

## Facts the code does not show

- Carrier webhooks are signed; the staging signing key is shared through the team vault, not the repository.
- `legacy/` is frozen and excluded from lint; do not refactor it.
