import os

from setuptools import find_packages, setup

VERSION = os.environ.get("RELEASE_VERSION", "0.0.0-placeholder")
REPO_NAME = "Goldziher/ai-rulez"

readme_path = os.path.join(os.path.dirname(__file__), "README.md")
try:
    with open(readme_path, encoding="utf-8") as f:
        long_description = f.read()
except FileNotFoundError:
    readme_path = os.path.join(os.path.dirname(__file__), "..", "..", "README.md")
    with open(readme_path, encoding="utf-8") as f:
        long_description = f.read()

setup(
    name="ai-rulez",
    version=VERSION,
    description="Author, generate, validate and govern rules, skills, plugins and MCP configuration for 52 AI coding assistants from one .ai-rulez/ source. Lock, sign and SBOM your agent configuration, review it with verified model evidence, and publish plugin bundles. CLI, MCP server and Go API.",
    long_description=long_description,
    long_description_content_type="text/markdown",
    author="Na'aman Hirschfeld",
    author_email="nhirschfeld@gmail.com",
    url=f"https://github.com/{REPO_NAME}",
    project_urls={
        "Homepage": "https://goldziher.github.io/ai-rulez/",
        "Documentation": "https://goldziher.github.io/ai-rulez/",
        "Bug Reports": f"https://github.com/{REPO_NAME}/issues",
        "Source": f"https://github.com/{REPO_NAME}",
        "Changelog": f"https://github.com/{REPO_NAME}/blob/main/CHANGELOG.md",
        "Funding": "https://github.com/sponsors/Goldziher",
    },
    keywords=[
        "ai-rules",
        "agents-md",
        "agent-skills",
        "agent-plugins",
        "claude-code",
        "cursor",
        "codex",
        "copilot",
        "mcp",
        "ai-governance",
        "sbom",
        "sigstore",
        "supply-chain",
        "llms-txt",
        "cli",
    ],
    license_expression="MIT AND Apache-2.0 AND Unicode-3.0",
    license_files=["LICENSE", "THIRD_PARTY_NOTICES.md", "LICENSES/*"],
    packages=find_packages(),
    install_requires=[],
    python_requires=">=3.11",
    entry_points={
        "console_scripts": [
            "ai-rulez=ai_rulez.cli:main",
        ],
    },
    classifiers=[
        "Development Status :: 5 - Production/Stable",
        "Intended Audience :: Developers",
        "Topic :: Software Development :: Code Generators",
        "Topic :: Software Development :: Quality Assurance",
        "Topic :: Security",
        "Topic :: System :: Software Distribution",
        "Topic :: Software Development :: Pre-processors",
        "Topic :: Text Processing :: General",
        "Topic :: Utilities",
        "Programming Language :: Python :: 3",
        "Programming Language :: Python :: 3 :: Only",
        "Programming Language :: Python :: 3.11",
        "Programming Language :: Python :: 3.12",
        "Programming Language :: Python :: 3.13",
        "Programming Language :: Python :: 3.14",
        "Programming Language :: Go",
        "Operating System :: MacOS",
        "Operating System :: Microsoft :: Windows",
        "Operating System :: POSIX :: Linux",
        "Environment :: Console",
    ],
)
