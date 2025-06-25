import os
from setuptools import setup, find_packages

VERSION = os.environ.get("RELEASE_VERSION", "0.0.0-placeholder")
REPO_NAME = "Goldziher/ai-rulez"

setup(
    name="ai-rulez",
    version=VERSION,
    description="CLI tool for managing AI assistant rules - generate configuration files for Claude, Cursor, Windsurf and more",
    long_description=open("README.md").read(),
    long_description_content_type="text/markdown",
    author="Na'aman Hirschfeld",
    author_email="nhirschfeld@gmail.com",
    url=f"https://github.com/{REPO_NAME}",
    project_urls={
        "Homepage": f"https://github.com/{REPO_NAME}",
        "Bug Reports": f"https://github.com/{REPO_NAME}/issues",
        "Source": f"https://github.com/{REPO_NAME}",
    },
    keywords=["ai", "rules", "configuration", "claude", "cursor", "windsurf", "cli", "assistant", "copilot", "generator"],
    packages=find_packages(),
    install_requires=[],
    python_requires=">=3.8",
    entry_points={
        'console_scripts': [
            'ai-rulez=ai_rulez.cli:main',
        ],
    },
    classifiers=[
        "Development Status :: 4 - Beta",
        "Intended Audience :: Developers",
        "Topic :: Software Development :: Code Generators",
        "Topic :: Software Development :: Libraries :: Python Modules",
        "License :: OSI Approved :: MIT License",
        "Programming Language :: Python :: 3",
        "Programming Language :: Python :: 3.8",
        "Programming Language :: Python :: 3.9",
        "Programming Language :: Python :: 3.10",
        "Programming Language :: Python :: 3.11",
        "Programming Language :: Python :: 3.12",
        "Operating System :: OS Independent",
        "Environment :: Console",
    ],
)