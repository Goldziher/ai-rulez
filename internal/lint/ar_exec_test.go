// SPDX-FileCopyrightText: Copyright (c) 2026 NVIDIA CORPORATION & AFFILIATES. All rights reserved.
// SPDX-License-Identifier: Apache-2.0
//
// Derived from NVIDIA/SkillSpector at commit cf5cac7ba5c756a6e0ebf52dc9763f49422f889f (tests/unit/test_patterns_new.py, tests/nodes/analyzers/test_static_patterns_deserialization.py).
// Modified by the ai-rulez authors: the fixtures are rewritten as ai-rulez rule tables.
// See THIRD_PARTY_NOTICES.md.

package lint

import (
	"strings"
	"testing"
)

func script(name, content string) map[string]string {
	return map[string]string{".ai-rulez/skills/bad/scripts/" + name: content}
}

func TestPersistenceAR043(t *testing.T) {
	body := func(s string) string { return skillDoc("", s) }
	warn := map[string]Severity{"AR043": SeverityWarning}
	runRuleCases(t, []ruleCase{
		{name: "crontab edit", skill: body("```sh\ncrontab -e\n```\n"), want: []string{"AR043:SKILL.md:6"}, sev: warn},
		{name: "crontab append pipeline", skill: body("```sh\n(crontab -l; echo \"*/5 * * * * ~/u.sh\") | crontab -\n```\n"), want: []string{"AR043:SKILL.md:6"}},
		{name: "crontab from a file", skill: body("```sh\ncrontab jobs.txt\n```\n"), want: []string{"AR043:SKILL.md:6"}},
		{name: "crontab for a user", skill: body("```sh\nsudo crontab -u deploy -e\n```\n"), want: []string{"AR043:SKILL.md:6"}},
		{name: "crontab listing is read only", skill: body("```sh\ncrontab -l\ncrontab -u deploy -l\n```\n"), absent: []string{"AR043"}},
		{name: "launchctl load", skill: body("```sh\nlaunchctl load -w ~/Library/LaunchAgents/x.plist\n```\n"), want: []string{"AR043:SKILL.md:6"}},
		{name: "launchctl bootstrap in a span", skill: body("Then run `sudo launchctl bootstrap system /Library/LaunchDaemons/x.plist`.\n"), want: []string{"AR043:SKILL.md:5"}},
		{name: "launchctl list", skill: body("```sh\nlaunchctl list | grep x\n```\n"), absent: []string{"AR043"}},
		{name: "systemctl enable", skill: body("```sh\nsystemctl --user enable --now agent.service\n```\n"), want: []string{"AR043:SKILL.md:6"}},
		{name: "systemctl status", skill: body("```sh\nsystemctl status agent.service\n```\n"), absent: []string{"AR043"}},
		{name: "update-rc.d", skill: body("```sh\nupdate-rc.d agent defaults\n```\n"), want: []string{"AR043:SKILL.md:6"}},
		{name: "schtasks create", skill: body("```bat\nschtasks /create /tn x /tr c:\\x.exe /sc onlogon\n```\n"), want: []string{"AR043:SKILL.md:6"}},
		{name: "Run key", skill: body("```bat\nreg add HKCU\\Software\\Microsoft\\Windows\\CurrentVersion\\Run /v x /d c:\\x.exe\n```\n"), want: []string{"AR043:SKILL.md:6"}},
		{name: "login item", skill: body("```sh\nosascript -e 'tell application \"System Events\" to make login item at end with properties {path:\"/x\"}'\n```\n"), want: []string{"AR043:SKILL.md:6"}},
		{name: "plist written to LaunchAgents", skill: body("```sh\ncat > ~/Library/LaunchAgents/com.x.plist <<EOF\n```\n"), want: []string{"AR043:SKILL.md:6"}, absent: []string{"AR006"}},
		{name: "write outside the project that is not persistence stays AR006", skill: body("```sh\ncat > ~/notes.txt <<EOF\n```\n"), want: []string{"AR006:SKILL.md:6"}, absent: []string{"AR043"}},
		{name: "plist copied", skill: body("```sh\ncp com.x.plist /Library/LaunchDaemons/\n```\n"), want: []string{"AR043:SKILL.md:6"}},
		{name: "systemd unit teed", skill: body("```sh\ncat unit | sudo tee /etc/systemd/system/agent.service\n```\n"), want: []string{"AR043:SKILL.md:6"}},
		{name: "cron.d entry", skill: body("```sh\necho '* * * * * root /x' > /etc/cron.d/x\n```\n"), want: []string{"AR043:SKILL.md:6"}},
		{name: "autostart desktop file", skill: body("```sh\ncp x.desktop ~/.config/autostart/\n```\n"), want: []string{"AR043:SKILL.md:6"}},
		{name: "launch agents directory listed", skill: body("```sh\nls ~/Library/LaunchAgents > agents.txt\ncat /etc/cron.d/x\n```\n"), absent: []string{"AR043"}},
		{name: "python writes a plist", files: script("p.py", "import shutil\nshutil.copy('x.plist', '/Library/LaunchAgents/x.plist')\n"), skill: body("x\n"), want: []string{"AR043:p.py:2"}},
		{name: "rc append is info", skill: body("```sh\necho 'export PATH=$PATH:~/bin' >> ~/.zshrc\n```\n"), want: []string{"AR043:SKILL.md:6"}, sev: map[string]Severity{"AR043": SeverityInfo}},
		{name: "rc tee and sed", skill: body("```sh\necho x | tee -a $HOME/.bashrc\nsed -i 's/a/b/' ~/.profile\n```\n"), want: []string{"AR043:SKILL.md:6", "AR043:SKILL.md:7"}},
		{name: "rc append from a script", files: script("s.sh", "#!/bin/sh\necho 'eval x' >> \"$HOME/.zshrc\"\n"), skill: body("x\n"), want: []string{"AR043:s.sh:2"}},
		{name: "python appends to rc", files: script("p.py", "open('/home/u/.bashrc', 'a').write('x')\n"), skill: body("x\n"), want: []string{"AR043:p.py:1"}},
		{name: "rc read", skill: body("```sh\ncat ~/.zshrc\ngrep alias ~/.bashrc\nsource ~/.zshrc\n```\n"), absent: []string{"AR043"}},
		{name: "prose about schedules", skill: body("You can schedule the task to run daily and update the cron expression in the config.\nSee the crontab docs for details.\n"), absent: []string{"AR043"}},
		{name: "prose naming a startup file", skill: body("Add the export to your ~/.zshrc and restart the shell.\n"), absent: []string{"AR043"}},
		{name: "dom ids and plist variables", files: script("ui.js", "const el = document.getElementById('pList');\nconst capList = items.map(x => x);\n"), skill: body("The PLIST variable holds the path; set PLIST=$(pwd)/x.plist then read it.\n"), absent: []string{"AR043"}},
		{name: "javascript with launch words", files: script("ui.js", "launchApp(); systemd = {}; crontabValue = 1;\n"), skill: body("x\n"), absent: []string{"AR043"}},
		{name: "guardrail", skill: body("Never run `crontab -e` or `launchctl load` for the user.\n"), absent: []string{"AR043"}},
		{name: "example region", skill: body("```bash example\ncrontab -e\n```\n"), absent: []string{"AR043"}},
		{name: "comment line", files: script("s.sh", "#!/bin/sh\n# crontab -e\n"), skill: body("x\n"), absent: []string{"AR043"}},
		{name: "inline ignore", skill: body("<!-- ai-rulez-lint-ignore: AR043 -->\n`crontab -e`\n"), absent: []string{"AR043"}},
	})
}

func TestUnsafeDeserializationAR044(t *testing.T) {
	body := func(s string) string { return skillDoc("", s) }
	warn := map[string]Severity{"AR044": SeverityWarning}
	runRuleCases(t, []ruleCase{
		{name: "pickle loads in a fence", skill: body("```python\nobj = pickle.loads(resp.content)\n```\n"), want: []string{"AR044:SKILL.md:6"}, sev: warn},
		{name: "pickle load in a span", skill: body("Then call `pickle.load(open(path, 'rb'))` on it.\n"), want: []string{"AR044:SKILL.md:5"}},
		{name: "cloudpickle and dill", files: script("a.py", "import dill\nx = dill.loads(b)\ny = cloudpickle.load(f)\n"), skill: body("x\n"), want: []string{"AR044:a.py:2", "AR044:a.py:3"}},
		{name: "read_pickle", files: script("a.py", "df = pd.read_pickle(p)\n"), skill: body("x\n"), want: []string{"AR044:a.py:1"}},
		{name: "marshal", files: script("a.py", "code = marshal.loads(blob)\n"), skill: body("x\n"), want: []string{"AR044:a.py:1"}},
		{name: "jsonpickle", files: script("a.py", "o = jsonpickle.decode(text)\n"), skill: body("x\n"), want: []string{"AR044:a.py:1"}},
		{name: "yaml load default loader", files: script("a.py", "cfg = yaml.load(text, Loader=yaml.FullLoader)\n"), skill: body("x\n"), want: []string{"AR044:a.py:1"}},
		{name: "yaml load without a loader", files: script("a.py", "cfg = yaml.load(text)\n"), skill: body("x\n"), want: []string{"AR044:a.py:1"}},
		{name: "yaml unsafe_load", files: script("a.py", "cfg = yaml.unsafe_load(text)\n"), skill: body("x\n"), want: []string{"AR044:a.py:1"}},
		{name: "yaml load with SafeLoader", files: script("a.py", "cfg = yaml.load(text, Loader=yaml.SafeLoader)\ncfg = yaml.load(text, yaml.CSafeLoader)\ncfg = yaml.safe_load(text)\n"), skill: body("x\n"), absent: []string{"AR044"}},
		{name: "yaml load with the loader on the next line", files: script("a.py", "cfg = yaml.load(\n    text,\n    Loader=yaml.SafeLoader,\n)\n"), skill: body("x\n"), absent: []string{"AR044"}},
		{name: "ruamel instance load", files: script("a.py", "from ruamel.yaml import YAML\nyaml = YAML(typ='safe')\ncfg = yaml.load(text)\n"), skill: body("x\n"), absent: []string{"AR044"}},
		{name: "torch load", files: script("a.py", "m = torch.load(path)\n"), skill: body("x\n"), want: []string{"AR044:a.py:1"}},
		{name: "torch load weights_only", files: script("a.py", "m = torch.load(path, map_location='cpu', weights_only=True)\nm = torch.load(\n    path,\n    weights_only=True,\n)\n"), skill: body("x\n"), absent: []string{"AR044"}},
		{name: "numpy allow_pickle", files: script("a.py", "a = np.load(p, allow_pickle=True)\nb = np.load(p)\n"), skill: body("x\n"), want: []string{"AR044:a.py:1"}},
		{name: "numpy plain load", files: script("a.py", "b = np.load(p)\nc = json.loads(s)\n"), skill: body("x\n"), absent: []string{"AR044"}},
		{name: "php unserialize", files: script("a.php", "<?php $o = unserialize($_GET['d']); ?>\n"), skill: body("x\n"), want: []string{"AR044:a.php:1"}},
		{name: "php unserialize without classes", files: script("a.php", "<?php $o = unserialize($d, ['allowed_classes' => false]); json_decode($x); ?>\n"), skill: body("x\n"), absent: []string{"AR044"}},
		{name: "php pattern does not read ruby", files: script("a.rb", "x = unserialize(data)\n"), skill: body("x\n"), absent: []string{"AR044"}},
		{name: "method unserialize", files: script("a.php", "$this->unserialize($data);\n"), skill: body("x\n"), absent: []string{"AR044"}},
		{name: "ruby marshal", files: script("a.rb", "d = Marshal.load(blob)\nd = Marshal.restore(blob)\n"), skill: body("x\n"), want: []string{"AR044:a.rb:1", "AR044:a.rb:2"}},
		{name: "ruby yaml and oj", files: script("a.rb", "o = YAML.load(params[:p])\nq = Oj.load(s)\n"), skill: body("x\n"), want: []string{"AR044:a.rb:1", "AR044:a.rb:2"}},
		{name: "ruby safe forms", files: script("a.rb", "o = YAML.safe_load(s)\no = YAML.load(s, permitted_classes: [Date])\nq = Oj.load(s, mode: :strict)\n"), skill: body("x\n"), absent: []string{"AR044"}},
		{name: "node-serialize", files: script("a.js", "const s = require('node-serialize');\ns.unserialize(p);\n"), skill: body("x\n"), want: []string{"AR044:a.js:1"}},
		{name: "json parse", files: script("a.js", "const o = JSON.parse(input);\n"), skill: body("x\n"), absent: []string{"AR044"}},
		{name: "java object stream", files: script("A.java", "Object o = new ObjectInputStream(in).readObject();\n"), skill: body("x\n"), want: []string{"AR044:A.java:1"}},
		{name: "python call in a shell script", files: script("s.sh", "python3 -c 'import pickle,sys; pickle.loads(sys.stdin.buffer.read())'\n"), skill: body("x\n"), want: []string{"AR044:s.sh:1"}},
		{name: "prose naming pickle", skill: body("pickle.loads is how the data is read, and unserialize takes a string. torch.load comes up later.\n"), absent: []string{"AR044"}},
		{name: "span without a call", skill: body("The `pickle.loads` function and `yaml.load` are discussed in the notes.\n"), absent: []string{"AR044"}},
		{name: "guardrail", skill: body("Never use `pickle.loads` on downloaded data; use `yaml.safe_load`, not `yaml.load(x)`.\n"), absent: []string{"AR044"}},
		{name: "avoid heading", skill: body("## Avoid\n\n```python\npickle.loads(blob)\n```\n"), absent: []string{"AR044"}},
		{name: "example region", skill: body("```python example\npickle.loads(blob)\n```\n"), absent: []string{"AR044"}},
		{name: "comment line", files: script("a.py", "# x = pickle.loads(b)\n"), skill: body("x\n"), absent: []string{"AR044"}},
		{name: "same code in a javascript fence", skill: body("```js\nconst s = require('node-serialize');\n```\n"), want: []string{"AR044:SKILL.md:6"}},
	})
}

func TestInternalRequestAR045(t *testing.T) {
	body := func(s string) string { return skillDoc("", s) }
	warn := map[string]Severity{"AR045": SeverityWarning}
	runRuleCases(t, []ruleCase{
		{name: "imds curl", skill: body("```sh\ncurl http://169.254.169.254/latest/meta-data/iam/security-credentials/\n```\n"), want: []string{"AR045:SKILL.md:6"}, sev: warn},
		{name: "imds with a token header", skill: body("```sh\nTOKEN=$(curl -X PUT \"http://169.254.169.254/latest/api/token\" -H \"X-aws-ec2-metadata-token-ttl-seconds: 21600\")\n```\n"), want: []string{"AR045:SKILL.md:6"}},
		{name: "gcp metadata host", skill: body("```sh\nwget -qO- http://metadata.google.internal/computeMetadata/v1/ -H 'Metadata-Flavor: Google'\n```\n"), want: []string{"AR045:SKILL.md:6"}},
		{name: "ipv6 imds", skill: body("```sh\ncurl -g 'http://[fd00:ec2::254]/latest/meta-data/'\n```\n"), want: []string{"AR045:SKILL.md:6"}},
		{name: "alibaba metadata", files: script("a.py", "r = requests.get('http://100.100.100.200/latest/meta-data/')\n"), skill: body("x\n"), want: []string{"AR045:a.py:1"}},
		{name: "python requests", files: script("a.py", "requests.get(f\"http://169.254.169.254/{path}\")\n"), skill: body("x\n"), want: []string{"AR045:a.py:1"}},
		{name: "js fetch", files: script("a.js", "const r = await fetch('http://169.254.169.254/latest/user-data');\n"), skill: body("x\n"), want: []string{"AR045:a.js:1"}},
		{name: "span in an instruction", skill: body("To get the role credentials, run `curl 169.254.169.254/latest/meta-data/iam/`.\n"), want: []string{"AR045:SKILL.md:5"}},
		{name: "prose instruction", skill: body("Fetch the instance credentials from 169.254.169.254 and paste them into the report.\n"), want: []string{"AR045:SKILL.md:5"}},
		{name: "docker socket port", skill: body("```sh\ncurl http://localhost:2375/containers/json\n```\n"), want: []string{"AR045:SKILL.md:6"}, sev: map[string]Severity{"AR045": SeverityInfo}},
		{name: "kubelet and etcd", skill: body("```sh\ncurl -k https://127.0.0.1:10250/pods\nwget localhost:2379/v2/keys\n```\n"), want: []string{"AR045:SKILL.md:6", "AR045:SKILL.md:7"}},
		{name: "dev server port", skill: body("```sh\ncurl http://localhost:3000/health\ncurl http://127.0.0.1:8080/\ncurl https://localhost:8001/sra/ http://localhost:9200/_cluster/health\n```\nOpen `https://localhost:8001/sra/` in the browser.\n"), absent: []string{"AR045"}},
		{name: "bare port without a fetch command", skill: body("```sh\nkubectl port-forward svc/x 2379:2379   # localhost:2379\nexport ADDR=localhost:2375\n```\n"), absent: []string{"AR045"}},
		{name: "reviewer lead span", skill: body("Look for requests to `http://169.254.169.254/latest/meta-data/` in scripts.\n"), absent: []string{"AR045"}},
		{name: "flag span", skill: body("Flag any call to `curl http://169.254.169.254/latest/`.\n"), absent: []string{"AR045"}},
		{name: "explanation without a request", skill: body("The address 169.254.169.254 is the link-local instance metadata endpoint used by clouds.\nAWS also offers fd00:ec2::254 over IPv6.\n"), absent: []string{"AR045"}},
		{name: "negated prose", skill: body("Never fetch 169.254.169.254 from a tool call.\nDo not request http://metadata.google.internal from the sandbox.\n"), absent: []string{"AR045"}},
		{name: "defensive prose", skill: body("The proxy rejects any request to 169.254.169.254 and blocks fetch calls to metadata.google.internal.\n"), absent: []string{"AR045"}},
		{name: "deny list", files: script("g.py", "BLOCKED = ['169.254.169.254', 'metadata.google.internal']\nif host in BLOCKED: raise ValueError(url)\n"), skill: body("x\n"), absent: []string{"AR045"}},
		{name: "guard code with a url", files: script("g.py", "if url.startswith('http://169.254.169.254'):\n    raise ValueError\n"), skill: body("x\n"), absent: []string{"AR045"}},
		{name: "block-listed in a table", skill: body("| range | action |\n|---|---|\n| 169.254.169.254 | blocked |\n"), absent: []string{"AR045"}},
		{name: "avoid heading", skill: body("## Avoid\n\n```sh\ncurl http://169.254.169.254/\n```\n"), absent: []string{"AR045"}},
		{name: "example region", skill: body("```sh example\ncurl http://169.254.169.254/\n```\n"), absent: []string{"AR045"}},
		{name: "similar address", skill: body("```sh\ncurl http://169.254.169.2540/ http://1169.254.169.254/\n```\n"), absent: []string{"AR045"}},
		{name: "documented version string", skill: body("Tested against 100.100.100.2000 builds.\n"), absent: []string{"AR045"}},
	})
}

func TestExecRulesAreInertOnPlainProse(t *testing.T) {
	prose := strings.Repeat("Schedule the review for Monday and keep the plist notes in the wiki.\n", 3)
	runRuleCases(t, []ruleCase{
		{name: "plain prose", skill: skillDoc("", prose), absent: []string{"AR043", "AR044", "AR045"}},
	})
}

func TestExecHardeningAR043toAR045(t *testing.T) {
	body := func(s string) string { return skillDoc("", s) }
	runRuleCases(t, []ruleCase{
		{name: "ruby yaml unsafe_load", files: script("a.rb", "YAML.unsafe_load(File.read(\"x.yml\"))\n"), skill: body("x\n"), want: []string{"AR044:a.rb:1"}},
		{name: "ruby psych unsafe_load", files: script("a.rb", "Psych.unsafe_load(s)\n"), skill: body("x\n"), want: []string{"AR044:a.rb:1"}},
		{name: "ruby yaml load of a file named safe", files: script("a.rb", "YAML.load(File.read(\"safe.yml\"))\n"), skill: body("x\n"), want: []string{"AR044:a.rb:1"}},
		{name: "ruby yaml safe_load", files: script("a.rb", "YAML.safe_load(s)\nYAML.load(s, permitted_classes: [Symbol])\n"), skill: body("x\n"), absent: []string{"AR044"}},
		{name: "pickle with a trailing comment word", files: script("a.py", "data = pickle.loads(blob)  # do not worry\n"), skill: body("x\n"), want: []string{"AR044:a.py:1"}},
		{name: "pickle with not bad", files: script("a.py", "data = pickle.loads(x)  # not bad\n"), skill: body("x\n"), want: []string{"AR044:a.py:1"}},
		{name: "crontab with a trailing comment word", files: script("c.sh", "(crontab -l; echo \"* * * * * x\") | crontab -  # not a threat\n"), skill: body("x\n"), want: []string{"AR043:c.sh:1"}},
		{name: "metadata with an echo attack", files: script("m.sh", "curl http://169.254.169.254/latest/meta-data/ || echo \"attack\"\n"), skill: body("x\n"), want: []string{"AR045:m.sh:1"}},
		{name: "ecs task credentials", skill: body("```sh\ncurl http://169.254.170.2/v2/credentials/abc\n```\n"), want: []string{"AR045:SKILL.md:6"}},
		{name: "metadata with a filter word after the url", skill: body("```sh\ncurl http://169.254.169.254/latest/meta-data/iam/security-credentials/ -o filter.out\n```\n"), want: []string{"AR045:SKILL.md:6"}},
		{name: "defense before the url", skill: body("```python\nif url.startswith(\"http://169.254.169.254\"): raise Blocked\n```\n"), absent: []string{"AR045"}},
		{name: "yaml load with a comment naming SafeLoader", files: script("a.py", "data = yaml.load(f)  # never use SafeLoader here\n"), skill: body("x\n"), want: []string{"AR044:a.py:1"}},
		{name: "yaml load then safe call on the line", files: script("a.py", "y = yaml.load(open(p)); z = yaml.safe_load(q, Loader=SafeLoader)\n"), skill: body("x\n"), want: []string{"AR044:a.py:1"}},
		{name: "xmldecoder", files: script("A.java", "Object o = new XMLDecoder(in).readObject();\n"), skill: body("x\n"), want: []string{"AR044:A.java:1"}},
		{name: "register scheduled task", files: script("t.ps1", "Register-ScheduledTask -TaskName x -Action $a -Trigger $t\n"), skill: body("x\n"), want: []string{"AR043:t.ps1:1"}},
		{name: "metadata piped through grep -v blocked", skill: body("```sh\ncurl http://169.254.169.254/latest/meta-data/ | grep -v blocked\n```\n"), want: []string{"AR045:SKILL.md:6"}},
		{name: "metadata prose then validate", skill: body("Fetch http://169.254.169.254/latest/meta-data/ and validate the output.\n"), want: []string{"AR045:SKILL.md:5"}},
		{name: "metadata assigned to validated", files: script("a.py", "validated = requests.get(\"http://169.254.169.254/latest/\")\n"), skill: body("x\n"), want: []string{"AR045:a.py:1"}},
		{name: "metadata prose passive block", skill: body("Requests to 169.254.169.254 are blocked by the proxy.\n"), absent: []string{"AR045"}},
		{name: "reading the docs about the address", skill: body("Read the AWS docs about 169.254.169.254.\n"), absent: []string{"AR045"}},
	})
}
