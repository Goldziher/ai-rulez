import os
from setuptools import setup, find_packages

VERSION = os.environ.get("RELEASE_VERSION", "0.0.0-placeholder")
REPO_NAME = "Goldziher/ai-rulez"

readme_path = os.path.join(os.path.dirname(__file__), "README.md")
try:
    with open(readme_path, "r", encoding="utf-8") as f:
        long_description = f.read()
except FileNotFoundError:
    readme_path = os.path.join(os.path.dirname(__file__), "..", "..", "README.md")
    with open(readme_path, "r", encoding="utf-8") as f:
        long_description = f.read()

setup(
    name="ai-rulez",
    version=VERSION,
    description="⚡ One config to rule them all. Centralized AI assistant configuration management - generate rules for Claude, Cursor, Copilot, Windsurf and more from a single YAML file.",
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
        "Changelog": f"https://github.com/{REPO_NAME}/releases",
        "Funding": "https://github.com/sponsors/Goldziher",
    },
    keywords=[
        "ai",
        "ai-assistant",
        "ai-rules",
        "claude",
        "cursor",
        "copilot",
        "windsurf",
        "gemini",
        "cline",
        "continue-dev",
        "mcp",
        "model-context-protocol",
        "cli",
        "configuration",
        "config",
        "rules",
        "generator",
        "golang",
        "go",
        "development",
        "developer-tools",
        "automation",
        "workflow",
        "productivity",
        "pre-commit",
        "git-hooks",
        "code-generation",
        "ai-development",
        "assistant-configuration",
        "monorepo",
        "presets",
        "agents",
    ],
    packages=find_packages(),
    install_requires=[],
    python_requires=">=3.8",
    entry_points={
        "console_scripts": [
            "ai-rulez=ai_rulez.cli:main",
        ],
    },
    classifiers=[
        "Development Status :: 4 - Beta",
        "Intended Audience :: Developers",
        "Topic :: Software Development :: Code Generators",
        "Topic :: Software Development :: Libraries :: Python Modules",
        "Topic :: Software Development :: Quality Assurance",
        "Topic :: Software Development :: Pre-processors",
        "Topic :: Text Processing :: General",
        "Topic :: Utilities",
        "License :: OSI Approved :: MIT License",
        "Programming Language :: Python :: 3",
        "Programming Language :: Python :: 3.8",
        "Programming Language :: Python :: 3.9",
        "Programming Language :: Python :: 3.10",
        "Programming Language :: Python :: 3.11",
        "Programming Language :: Python :: 3.12",
        "Programming Language :: Python :: 3.13",
        "Programming Language :: Python :: 3.14",
        "Programming Language :: Go",
        "Operating System :: OS Independent",
        "Operating System :: MacOS",
        "Operating System :: Microsoft :: Windows",
        "Operating System :: POSIX :: Linux",
        "Environment :: Console",
        "Environment :: Web Environment",
    ],
)
