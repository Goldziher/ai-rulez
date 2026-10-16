import json, pathlib, sys

schema = json.loads((pathlib.Path(__file__).parent.parent / "references" / "schema.json").read_text())
doc = json.loads(open(sys.argv[1]).read())
print(schema["type"] == "object" and isinstance(doc, dict))
