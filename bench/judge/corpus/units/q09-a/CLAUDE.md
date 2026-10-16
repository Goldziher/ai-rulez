# Inventory service

Run tests with `make test`. Lint with `make lint`.

## How to cut a release

1. Make sure main is green in CI.
2. Run `git fetch --tags` and find the previous tag.
3. Run `make changelog` and review the generated entries.
4. Edit `VERSION` to the new number.
5. Commit with the message `Release <version>`.
6. Run `git tag v<version>`.
7. Push the commit and the tag.
8. Wait for the `publish` workflow to finish.
9. Open the GitHub release page and paste the changelog.
10. Post the release link in the team channel.
11. Close the milestone.
12. Delete the release branch if one was used.
