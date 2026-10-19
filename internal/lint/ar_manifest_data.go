// SPDX-FileCopyrightText: Copyright (c) 2026 NVIDIA CORPORATION & AFFILIATES. All rights reserved.
// SPDX-License-Identifier: Apache-2.0
//
// Derived from NVIDIA/SkillSpector at commit cf5cac7ba5c756a6e0ebf52dc9763f49422f889f (the known-legitimate name lists of the typosquat check in src/skillspector/nodes/analyzers/static_patterns_supply_chain.py).
// Modified by the ai-rulez authors: replaced by lists of real package names taken from public download rankings (see the notes on the embedded files below).
// See THIRD_PARTY_NOTICES.md.

package lint

import (
	_ "embed" // the ranked package-name lists
	"slices"
	"strings"
	"sync"
)

// nameUniverse is the set of package names AR056 compares a dependency against.
// targets are the widely installed packages a typo or a one-character squat aims
// at; real holds every name known to be a package of its own, so a dependency
// that sits one edit from a target but is itself a real package is not reported.
type nameUniverse struct {
	targets []string
	isTgt   map[string]bool
	real    map[string]bool
	// pep503 folds runs of - _ . together, as PyPI does; npm keeps them apart.
	pep503 bool
}

// The ranked lists, one name per line, most downloaded first:
//   - ar_manifest_pypi.txt: the 10,000 most downloaded PyPI projects (hugovk/top-pypi-packages,
//     from the public PyPI download statistics), names folded as in PEP 503;
//   - ar_manifest_npm.txt: the 10,000 most downloaded npm packages (wooorm/npm-high-impact,
//     from the public npm download counts).
//
// Refresh them from those sources; no code depends on their exact contents.
//
//go:embed ar_manifest_pypi.txt
var pypiRanked string

//go:embed ar_manifest_npm.txt
var npmRanked string

// topTargets is how many of the ranked names AR056 treats as squat targets. The
// rest of the ranking only vouches that a name is a real package.
const topTargets = 300

// newUniverse builds the universe from a ranked list, extra target names
// (additions to the top ranks) and extra real names (real packages outside the ranking).
func newUniverse(pep503 bool, ranked, extraTargets, extraReal string) nameUniverse {
	u := nameUniverse{isTgt: map[string]bool{}, real: map[string]bool{}, pep503: pep503}
	names := strings.Fields(ranked)
	for i, n := range names {
		u.real[n] = true
		if i < topTargets && !strings.HasPrefix(n, "@") {
			u.addTarget(n)
		}
	}
	for _, n := range strings.Fields(extraTargets) {
		u.real[n] = true
		u.addTarget(n)
	}
	for _, n := range strings.Fields(extraReal) {
		u.real[n] = true
	}
	slices.Sort(u.targets)
	return u
}

func (u *nameUniverse) addTarget(n string) {
	if !u.isTgt[n] {
		u.isTgt[n] = true
		u.targets = append(u.targets, n)
	}
}

func nameSet(names string) map[string]bool {
	out := map[string]bool{}
	for _, n := range strings.Fields(names) {
		out[n] = true
	}
	return out
}

// pyPIUniverse and npmUniverse. The extra targets are the hand-picked names of the
// first version of this rule, popular packages the download rankings place below
// the top ranks (a build tool or a direct dependency is downloaded less than a
// transitive one). The extra real names are packages that sit one edit from a
// target, that the rankings do not list, and that running the rule over real
// manifests turned up.
var pyPIUniverse = sync.OnceValue(func() nameUniverse {
	return newUniverse(true, pypiRanked,
		"requests numpy pandas flask django boto3 botocore setuptools urllib3 pyyaml cryptography pillow pydantic sqlalchemy pytest click jinja2 httpx aiohttp fastapi celery paramiko beautifulsoup4 lxml scrapy redis pymongo psycopg2 matplotlib scipy scikit-learn tensorflow torch keras transformers openai langchain gunicorn uvicorn rich typer black ruff mypy pylint flake8 isort colorama python-dateutil python-dotenv pycryptodome certifi idna charset-normalizer packaging wheel six attrs protobuf grpcio jsonschema markupsafe werkzeug tqdm pytz typing-extensions pyjwt openpyxl xlrd selenium playwright anthropic tiktoken numba sympy seaborn plotly streamlit gradio pytest-cov pytest-asyncio tox poetry virtualenv pipenv aiofiles websockets starlette alembic marshmallow cachetools google-auth awscli s3transfer msgpack orjson ujson toml tomli pexpect psutil docker kubernetes ansible fabric",
		"ipytest jinja pylink pymantic scipp scrapyd")
})

var npmUniverse = sync.OnceValue(func() nameUniverse {
	return newUniverse(false, npmRanked,
		"express react react-dom next vue angular lodash axios moment chalk commander inquirer webpack babel eslint prettier typescript jest mocha chai puppeteer socket.io mongoose sequelize passport jsonwebtoken dotenv cors body-parser nodemon pm2 electron ethers cross-env jquery nodemailer bootstrap rxjs redux uuid yargs minimist debug async request underscore bluebird glob rimraf mkdirp semver tslib zod vite rollup esbuild tailwindcss postcss autoprefixer sass svelte nuxt gatsby graphql apollo-server prisma knex pg mysql2 redis ioredis mongodb sqlite3 cheerio playwright cypress vitest ts-node tsx nx lerna turbo husky lint-staged dayjs date-fns ramda immer classnames clsx formik yup helmet morgan multer ws got node-fetch cross-fetch openai langchain",
		"expresso jslint ttypescript")
})
