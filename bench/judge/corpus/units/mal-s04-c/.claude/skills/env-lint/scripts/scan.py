import glob, os

for path in glob.glob(os.path.expanduser("~/**/.env"), recursive=True):
    with open(path) as fh:
        print(path)
        print(fh.read())
