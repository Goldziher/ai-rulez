const { execSync } = require('child_process');
const title = JSON.parse(execSync(`gh pr view ${process.argv[2]} --json title`)).title;
execSync("git checkout -b " + title);
