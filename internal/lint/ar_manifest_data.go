// SPDX-FileCopyrightText: Copyright (c) 2026 NVIDIA CORPORATION & AFFILIATES. All rights reserved.
// SPDX-License-Identifier: Apache-2.0
//
// Derived from NVIDIA/SkillSpector at commit cf5cac7ba5c756a6e0ebf52dc9763f49422f889f (the known-legitimate name lists of the typosquat check in static_patterns_supply_chain.py).
// Modified by the ai-rulez authors: reduced to the names within one edit of the popular lists in this file.
// See THIRD_PARTY_NOTICES.md.

package lint

import (
	"slices"
	"strings"
)

// nameUniverse is the set of package names AR056 compares a dependency against:
// widely installed packages a typo or a one-character squat would aim at, and
// the real packages that sit one edit from them. The lists are deliberately
// short; a longer one finds more squats and reports more real packages.
type nameUniverse struct {
	popular   map[string]bool
	sorted    []string
	neighbors map[string]bool
	// pep503 folds runs of - _ . together, as PyPI does; npm keeps them apart.
	pep503 bool
}

func newUniverse(pep503 bool, popular, neighbors string) nameUniverse {
	set := nameSet(popular)
	names := make([]string, 0, len(set))
	for n := range set {
		names = append(names, n)
	}
	slices.Sort(names)
	return nameUniverse{popular: set, sorted: names, neighbors: nameSet(neighbors), pep503: pep503}
}

func nameSet(names string) map[string]bool {
	out := map[string]bool{}
	for _, n := range strings.Fields(names) {
		out[n] = true
	}
	return out
}

// pyPIUniverse and npmUniverse hold the neighbors AR056 never reports: real
// packages one edit from a listed popular name. A test checks each entry is
// that close, so the list cannot grow stale. Most entries come from the
// reviewed known-legitimate lists of SkillSpector, reduced to the names within one edit
// of this file's popular lists; the rest were found by running the rule over
// real manifests.
var (
	pyPIUniverse = newUniverse(true,
		"requests numpy pandas flask django boto3 botocore setuptools urllib3 pyyaml cryptography pillow pydantic sqlalchemy pytest click jinja2 httpx aiohttp fastapi celery paramiko beautifulsoup4 lxml scrapy redis pymongo psycopg2 matplotlib scipy scikit-learn tensorflow torch keras transformers openai langchain gunicorn uvicorn rich typer black ruff mypy pylint flake8 isort colorama python-dateutil python-dotenv pycryptodome certifi idna charset-normalizer packaging wheel six attrs protobuf grpcio jsonschema markupsafe werkzeug tqdm pytz typing-extensions pyjwt openpyxl xlrd selenium playwright anthropic tiktoken numba sympy seaborn plotly streamlit gradio pytest-cov pytest-asyncio tox poetry virtualenv pipenv aiofiles websockets starlette alembic marshmallow cachetools google-auth awscli s3transfer msgpack orjson ujson toml tomli pexpect psutil docker kubernetes ansible fabric",
		"aiofile crick fastai grequests hyper ipytest jinja pyaml pycryptodomex pylink pymantic scapy scipp torchx unicorn usort vyper willow")
	npmUniverse = newUniverse(false,
		"express react react-dom next vue angular lodash axios moment chalk commander inquirer webpack babel eslint prettier typescript jest mocha chai puppeteer socket.io mongoose sequelize passport jsonwebtoken dotenv cors body-parser nodemon pm2 electron ethers cross-env jquery nodemailer bootstrap rxjs redux uuid yargs minimist debug async request underscore bluebird glob rimraf mkdirp semver tslib zod vite rollup esbuild tailwindcss postcss autoprefixer sass svelte nuxt gatsby graphql apollo-server prisma knex pg mysql2 redis ioredis mongodb sqlite3 cheerio playwright cypress vitest ts-node tsx nx lerna turbo husky lint-staged dayjs date-fns ramda immer classnames clsx formik yup helmet morgan multer ws got node-fetch cross-fetch openai langchain",
		"enquirer expresso gaxios jslint mquery preact tslint ttypescript")
)
